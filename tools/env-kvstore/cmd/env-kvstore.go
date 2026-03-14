package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/cognitotoken"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/kvstore"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	cliConfig, err := parseCLIConfig()
	if err != nil {
		slog.Error("Error parsing CLI input.", "error", err)
		os.Exit(1)
	}

	if err := cliConfig.Validate(); err != nil {
		slog.Error("Invalid configuration.", "error", err)
		os.Exit(1)
	}

	if err := run(ctx, cliConfig); err != nil {
		slog.Error("Error running env-kvstore.", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, config config.Config) error {
	tokenExchanger := cognitotoken.NewTokenExchanger(ctx, &config.Cognito, &config.GHA)
	awsCredentialsProvider, err := tokenExchanger.CreateProvider()
	if err != nil {
		return fmt.Errorf("error creating Cognito role credentials provider: %w", err)
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion(config.SecretsManager.Region),
		awscfg.WithCredentialsProvider(awsCredentialsProvider),
	)
	if err != nil {
		return fmt.Errorf("error creating AWS configuration: %w", err)
	}

	// Validate AWS credentials by making a call to STS GetCallerIdentity before proceeding with Secrets Manager calls.
	// Need to ensure credentials are valid and that tokenExchanger.Claims is populated with values from GHA JWT.
	stsClient := sts.NewFromConfig(awsCfg)
	identityOutput, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("error validating AWS credentials with STS GetCallerIdentity: %w", err)
	}
	slog.Info("Successfully authenticated to AWS account.", "account", aws.ToString(identityOutput.Account), "arn", aws.ToString(identityOutput.Arn))

	slog.Info("Retrieving secrets from AWS Secrets Manager and setting environment variables.")
	_, err = kvstore.NewSecretsManagerValueProvider(ctx, awsCfg, config.SecretsManager, config.Values.Items, tokenExchanger.Claims)
	if err != nil {
		return fmt.Errorf("error creating Secrets Manager value provider: %w", err)
	}

	return nil
}

func parseCLIConfig() (config.Config, error) {
	cfg := config.NewFromEnv()

	flag.StringVar(&cfg.SecretsManager.AccountID, "secrets-manager-account-id", cfg.SecretsManager.AccountID, "AWS account ID where Secrets Manager secrets are located.")
	flag.StringVar(&cfg.SecretsManager.Region, "secrets-manager-region", cfg.SecretsManager.Region, "AWS region where Secrets Manager secrets are located.")
	flag.StringVar(&cfg.Cognito.AccountID, "cognito-account-id", cfg.Cognito.AccountID, "AWS account ID where Cognito is located.")
	flag.StringVar(&cfg.Cognito.IdentityPoolID, "cognito-identity-pool-id", cfg.Cognito.IdentityPoolID, "Cognito identity pool ID.")
	flag.StringVar(&cfg.Cognito.RoleARN, "cognito-role-arn", cfg.Cognito.RoleARN, "Cognito role ARN.")
	flag.StringVar(&cfg.Values.ValuesInput, "values", cfg.Values.ValuesInput, "Values to retrieve from KVStore and set as environment variables. CSV: environment variable name, value type (variable|secret), value source (repo|env), name of AWS Secrets Manager secret")
	flag.StringVar(&cfg.GHA.IDTokenRequestToken, "gha-id-token-request-token", cfg.GHA.IDTokenRequestToken, "GitHub Actions ID token request token for retrieving OIDC token to authenticate with Cognito when AWS credentials are not provided.")
	flag.StringVar(&cfg.GHA.IDTokenRequestURL, "gha-id-token-request-url", cfg.GHA.IDTokenRequestURL, "GitHub Actions ID token request URL for retrieving OIDC token to authenticate with Cognito when AWS credentials are not provided.")
	flag.StringVar(&cfg.GHA.EnterpriseName, "github-enterprise-name", "teleport", "GitHub Enterprise name. Used to validate JWT issuer and generate the AWS OIDC provider.")

	flag.Parse()

	return cfg, nil
}
