package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/cognitotoken"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	githubToken := kingpin.Flag("github-token", "GitHub token to identify the workflow accessing KVStore values.").Envar("INPUT_GITHUB-TOKEN").String()
	secretsManagerAccountID := kingpin.Flag("secrets-manager-account-id", "AWS account ID where Secrets Manager secrets are located.").Envar("INPUT_SECRETS-MANAGER-ACCOUNT-ID").String()
	secretsManagerRegion := kingpin.Flag("secrets-manager-region", "AWS region where Secrets Manager secrets are located.").Envar("INPUT_SECRETS-MANAGER-REGION").Default("us-west-2").String()
	cognitoAccountID := kingpin.Flag("cognito-account-id", "AWS account ID where Cognito is located.").Envar("INPUT_COGNITO-ACCOUNT-ID").String()
	cognitoIdentityPoolID := kingpin.Flag("cognito-identity-pool-id", "Cognito identity pool ID.").Envar("INPUT_COGNITO-IDENTITY-POOL-ID").String()
	cognitoRegion := kingpin.Flag("cognito-region", "AWS region where Cognito is located.").Envar("INPUT_COGNITO-REGION").Default("us-west-2").String()
	cognitoRoleARN := kingpin.Flag("cognito-role-arn", "Cognito role ARN.").Envar("INPUT_COGNITO-ROLE-ARN").String()
	values := kingpin.Flag("values", "Values to retrieve from KVStore and set as environment variables. CSV: environment variable name, value type (variable|secret), value source (repo|env), name of AWS Secrets Manager secret").Envar("INPUT_VALUES").String()
	ghaIDTokenRequestToken := kingpin.Flag("gha-id-token-request-token", "GitHub Actions ID token request token for retrieving OIDC token to authenticate with Cognito when AWS credentials are not provided.").Envar("ACTIONS_ID_TOKEN_REQUEST_TOKEN").String()
	ghaIDTokenRequestURL := kingpin.Flag("gha-id-token-request-url", "GitHub Actions ID token request URL for retrieving OIDC token to authenticate with Cognito when AWS credentials are not provided.").Envar("ACTIONS_ID_TOKEN_REQUEST_URL").String()

	kingpin.Parse()

	cfg := config.NewWithEnv()

	cliConfig := config.Config{
		Cognito: config.CognitoConfig{
			AccountID:      aws.ToString(cognitoAccountID),
			IdentityPoolID: aws.ToString(cognitoIdentityPoolID),
			Region:         aws.ToString(cognitoRegion),
			RoleARN:        aws.ToString(cognitoRoleARN),
		},
		SecretsManager: config.SecretsManagerConfig{
			AccountID: aws.ToString(secretsManagerAccountID),
			Region:    aws.ToString(secretsManagerRegion),
		},
		Values: aws.ToString(values),
		GHA: config.GHAConfig{
			IDTokenRequestToken: aws.ToString(ghaIDTokenRequestToken),
			IDTokenRequestURL:   aws.ToString(ghaIDTokenRequestURL),
			GitHubToken:         aws.ToString(githubToken),
		},
	}

	cfg.Merge(&cliConfig)
	if err := cfg.Validate(); err != nil {
		slog.Error("Invalid configuration.", "error", err)
		os.Exit(1)
	}

	if err := run(ctx, *cfg); err != nil {
		slog.Error("Error running env-kvstore.", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, config config.Config) error {
	tokenExchanger := cognitotoken.NewTokenExchanger(ctx, &config.Cognito, &config.GHA)
	provider, err := tokenExchanger.CreateProvider()
	if err != nil {
		return fmt.Errorf("error creating Cognito role credentials provider: %w", err)
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(config.SecretsManager.Region),
		awscfg.WithCredentialsProvider(provider),
	)
	if err != nil {
		return fmt.Errorf("error creating AWS configuration: %w", err)
	}

	stsClient := sts.NewFromConfig(awsCfg)
	identityOutput, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("error validating AWS credentials with STS GetCallerIdentity: %w", err)
	}
	slog.Info("Successfully authenticated to AWS account.", "account", aws.ToString(identityOutput.Account), "arn", aws.ToString(identityOutput.Arn))

	// TODO: implement:
	//  retrieve from Secrets Manager - environment specific values overwrite repo-level values
	//  set environment variables for subsequent steps
	//  mask secret values

	return nil
}
