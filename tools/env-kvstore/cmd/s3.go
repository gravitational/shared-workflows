package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/libs/github/actions"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/kvstore"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	githubMigrationStepName = "Collect Existing GHA Values for Migration"
)

type migrationConfigGetter interface {
	GetMigrationConfig() (kvstore.MigrationConfig, bool)
}

type MigrationUploader struct {
	awsConfig               aws.Config
	ghaClaims               config.GHAClaims
	migrationConfig         migrationConfigGetter
}

func generateS3Key(ghaClaims config.GHAClaims) (string, error) {
	if ghaClaims.Enterprise == "" || ghaClaims.Repository == "" || ghaClaims.RunID == "" || ghaClaims.Workflow == "" {
		return "", fmt.Errorf("missing required GHA claims: enterprise=%s, repository=%s, run_id=%s, workflow=%s", ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.RunID, ghaClaims.Workflow)
	}
	env := ghaClaims.Environment
	if env == "" {
		env = "__noenv__"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.Workflow, env, fmt.Sprintf("%s.json", ghaClaims.RunID)), nil
}

func (u *MigrationUploader) Upload(ctx context.Context) error {
	migrationConfig, ok := u.migrationConfig.GetMigrationConfig()
	if !ok {
		migrationSummary("Migration configuration not found. Skipping collection of values.", true)
		slog.Info("No valid migration configuration found, skipping upload to S3.")
		return nil
	}

	bucket := migrationConfig.S3Bucket
	content, err := json.Marshal(migrationConfig)
	if err != nil {
		return fmt.Errorf("marshal migration config: %w", err)
	}

	objectKey, err := generateS3Key(u.ghaClaims)
	if err != nil {
		return fmt.Errorf("generate S3 key: %w", err)
	}

	slog.Info("Uploading existing GitHub Actions values to S3.", "bucket", bucket, "key", objectKey, "bytes", len(content))

	s3Client := s3.NewFromConfig(u.awsConfig)
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(objectKey),
		Body:          bytes.NewReader(content),
		ContentLength: aws.Int64(int64(len(content))),
		ContentType:   aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("upload migration config to s3://%s/%s: %w", bucket, objectKey, err)
	}

	slog.Info("Uploaded existing GitHub Actions values to S3.", "bucket", bucket, "key", objectKey, "bytes", len(content))
	migrationSummary(fmt.Sprintf("Successfully uploaded existing GitHub Actions values to s3://%s/%s", bucket, objectKey), true)
	return nil
}

func migrationSummary(msg string, success bool) {
	result := actions.SummaryResultSuccess
	if !success {
		result = actions.SummaryResultFailure
	}
	actions.AddSummary(githubMigrationStepName, actions.SummaryRow{
		Result: result,
		Msg:    msg,
	})
}
