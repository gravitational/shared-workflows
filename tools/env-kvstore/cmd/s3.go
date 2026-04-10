package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const existingValuesUploadFile = "existing_values_s3_upload.json"

func generateS3Key(ghaClaims config.GHAClaims) (string, error) {
	if ghaClaims.Enterprise == "" || ghaClaims.Repository == "" || ghaClaims.RunID == "" || ghaClaims.Workflow == "" {
		return "", fmt.Errorf("missing required GHA claims: enterprise=%s, repository=%s, run_id=%s, workflow=%s", ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.RunID, ghaClaims.Workflow)
	}
	env := ghaClaims.Environment
	if env == "" {
		env = "__noenv__"
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.Workflow, env, fmt.Sprintf("%s.json",ghaClaims.RunID)), nil
}

func UploadToS3(ctx context.Context, awsConfig aws.Config, ghaClaims config.GHAClaims, bucket string) error {
	inputFile, err := os.Open(existingValuesUploadFile)
	if err != nil {
		return fmt.Errorf("open %s: %w", existingValuesUploadFile, err)
	}
	defer inputFile.Close()

	fileInfo, err := inputFile.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", existingValuesUploadFile, err)
	}

	objectKey, err := generateS3Key(ghaClaims)
	if err != nil {
		return fmt.Errorf("generate S3 key: %w", err)
	}

	slog.Info("Uploading existing GitHub Actions values to S3.", "bucket", bucket, "key", objectKey, "bytes", fileInfo.Size())

	s3Client := s3.NewFromConfig(awsConfig)
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(objectKey),
		Body:          inputFile,
		ContentLength: aws.Int64(fileInfo.Size()),
		ContentType:   aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("upload %s to s3://%s/%s: %w", existingValuesUploadFile, bucket, objectKey, err)
	}

	if _, err := inputFile.Seek(0, io.SeekStart); err == nil {
		slog.Info("Uploaded existing GitHub Actions values to S3.", "bucket", bucket, "key", objectKey, "bytes", fileInfo.Size())
	}

	return nil
}
