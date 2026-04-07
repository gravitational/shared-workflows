package kvstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/libs/github/actions"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const (
	repoARNFormat = "arn:aws:secretsmanager:%s:%s:secret:%s/repo/%s"
	envARNFormat  = "arn:aws:secretsmanager:%s:%s:secret:%s/repo/%s/env/%s"

	githubStepName = "Retrieve values from AWS Secrets Manager"
)

type SecretsManagerValueProvider struct {
	awsConfig aws.Config
	smConfig  config.SecretsManagerConfig
	// valuesConfig is the list of environment variables to retrieve from Secrets Manager
	valuesConfig []config.EnvValue
	// ghaClaims stores the parsed claims from the GHA JWT - used to construct ARNs for Secrets Manager and for naming the AWS session
	ghaClaims config.GHAClaims
	// secrets is the KV store for secret values retrieved from Secrets Manager, environment-specific values have precedence
	secrets KVStoreStringReader
	// variables is the KV store for variable values retrieved from Secrets Manager, environment-specific values have precedence
	variables KVStoreStringReader
	// ghaEnvValues contains the desired environment variables with their corresponding values retrieved from Secrets Manager
	ghaEnvValues []ghaEnvValue
}

type ghaEnvValue struct {
	VarName   string
	ValueType string
	Value     string
}

type arnTemplateFields struct {
	region      string
	accountID   string
	enterprise  string
	repo        string
	environment string
}

func repoSecretARN(valueType string, arnDetails arnTemplateFields) string {
	return fmt.Sprintf(
		fmt.Sprintf(
			"%s/%s",
			repoARNFormat,
			valueType),
		arnDetails.region,
		arnDetails.accountID,
		arnDetails.enterprise,
		arnDetails.repo)
}

func envSecretARN(valueType string, arnDetails arnTemplateFields) string {
	if arnDetails.environment == "" {
		return ""
	}
	return fmt.Sprintf(
		fmt.Sprintf("%s/%s",
			envARNFormat,
			valueType),
		arnDetails.region,
		arnDetails.accountID,
		arnDetails.enterprise,
		arnDetails.repo,
		arnDetails.environment)
}

func NewSecretsManagerValueProvider(awsConfig aws.Config, smConfig config.SecretsManagerConfig, ghaClaims config.GHAClaims, valuesConfig []config.EnvValue) *SecretsManagerValueProvider {
	return &SecretsManagerValueProvider{
		awsConfig:    awsConfig,
		smConfig:     smConfig,
		ghaClaims:    ghaClaims,
		valuesConfig: valuesConfig,
	}
}

// SetEnvValuesForGitHubActions sets the environment variables in GitHub Actions for subsequent
// steps to consume by appending them to the GITHUB_ENV file.
func (s *SecretsManagerValueProvider) SetEnvValuesForGitHubActions(ctx context.Context) error {
	if s.ghaEnvValues == nil {
		if err := s.populateEnvValues(ctx); err != nil {
			return fmt.Errorf("error populating environment variable values: %w", err)
		}
	}
	s.MaskAllSecretValues()

	envValues := make(map[string]string, len(s.ghaEnvValues))
	for _, v := range s.ghaEnvValues {
		envValues[v.VarName] = v.Value
	}

	if err := actions.WriteGithubEnv(envValues); err != nil {
		actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
			Result: actions.SummaryResultFailure,
			Msg:    fmt.Sprintf("Failed to set environment variables for GitHub Actions: %v", err),
			FailureCount: 1,
		})
		return fmt.Errorf("error appending environment variable definitions to GITHUB_ENV file: %w", err)
	}
	actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
		Result: actions.SummaryResultSuccess,
		Msg:    "Environment variables set successfully for GitHub Actions",
		SuccessCount: len(envValues),
	})

	return nil
}

// MaskAllSecretValues iterates through the values and adds a mask for any value of type "secret" so that they are not exposed in GitHub Actions logs if printed by subsequent steps.
func (s *SecretsManagerValueProvider) MaskAllSecretValues() {
	fmt.Println("\n::group::Masking secret values in GitHub Actions logs")
	secrets := make([]string, 0, len(s.ghaEnvValues))
	for _, v := range s.ghaEnvValues {
		if v.ValueType == "secret" && v.Value != "" {
			secrets = append(secrets, v.Value)
		}
	}
	actions.MaskSecretValues(secrets)
	defer fmt.Println("::endgroup::")
}

func (s *SecretsManagerValueProvider) populateEnvValues(ctx context.Context) error {
	if s.secrets == nil || s.variables == nil {
		if err := s.initializeStores(ctx); err != nil {
			return fmt.Errorf("error initializing stores: %w", err)
		}
	}
	var secretCount, secretErrCount, variableCount, variableErrCount int
	for _, v := range s.valuesConfig {
		var value string
		var ok bool
		if v.ValueType == "secret" {
			value, ok = s.secrets.GetValue(v.LookupKey())
			if !ok {
				slog.Warn("secret value not found in Secrets Manager for env var", "envVar", v.EnvVar, "keyOverride", v.KeyOverride)
				secretErrCount++
				continue
			}
			secretCount++
		} else if v.ValueType == "variable" {
			value, ok = s.variables.GetValue(v.LookupKey())
			if !ok {
				slog.Warn("variable value not found in Secrets Manager for env var", "envVar", v.EnvVar, "keyOverride", v.KeyOverride)
				variableErrCount++
				continue
			}
			variableCount++
		}
		s.ghaEnvValues = append(s.ghaEnvValues, ghaEnvValue{
			VarName:   v.EnvVar,
			ValueType: v.ValueType,
			Value:     value,
		})
	}
	stepResult := actions.SummaryRowWithCounts{
		Result:       actions.SummaryResultSuccess,
		Msg:          fmt.Sprintf("Populated %d secret values and %d variable values (%d errors)", secretCount, variableCount, secretErrCount+variableErrCount),
		SuccessCount: secretCount + variableCount,
		FailureCount: secretErrCount + variableErrCount,
	}
	if secretErrCount+variableErrCount > 0 {
		stepResult.Result = actions.SummaryResultFailure
	}
	actions.AddSummary(githubStepName, stepResult)
	if stepResult.Result == actions.SummaryResultFailure {
		return fmt.Errorf("failed to populate all environment variable values: %d secrets and %d variables were not found in Secrets Manager", secretErrCount, variableErrCount)
	}
	return nil
}

func (s *SecretsManagerValueProvider) initializeStores(ctx context.Context) error {
	arnDetails := arnTemplateFields{
		region:      s.smConfig.Region,
		accountID:   s.smConfig.AccountID,
		enterprise:  s.ghaClaims.Enterprise,
		repo:        s.ghaClaims.Repository,
		environment: s.ghaClaims.Environment,
	}

	repoSecretsARN := repoSecretARN("secrets", arnDetails)
	repoVariablesARN := repoSecretARN("variables", arnDetails)
	envSecretsARN := envSecretARN("secrets", arnDetails)
	envVariablesARN := envSecretARN("variables", arnDetails)

	slog.Debug("Creating secrets store", "repoSecretsARN", repoSecretsARN, "envSecretsARN", envSecretsARN)
	secrets, err := s.repoOrEnvStoreFromSecretARNs(ctx, repoSecretsARN, envSecretsARN)
	if err != nil {
		if !errors.As(err, &envStoreNotFoundError{}) {
			s.emitStoreInitFailureSummary(err, "secrets")
			return err
		}
		s.emitEnvStoreWarning("secrets")
	}

	s.secrets = secrets
	actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
		Result:       actions.SummaryResultSuccess,
		Msg:          "Secrets store created successfully",
		SuccessCount: 1,
	})

	slog.Debug("Creating variables store", "repoVariablesARN", repoVariablesARN, "envVariablesARN", envVariablesARN)
	variables, err := s.repoOrEnvStoreFromSecretARNs(ctx, repoVariablesARN, envVariablesARN)
	if err != nil {
		if !errors.As(err, &envStoreNotFoundError{}) {
			s.emitStoreInitFailureSummary(err, "variables")
			return err
		}
		s.emitEnvStoreWarning("variables")
	}

	s.variables = variables
	actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
		Result:       actions.SummaryResultSuccess,
		Msg:          "Variables store created successfully",
		SuccessCount: 1,
	})

	return nil
}

func (s *SecretsManagerValueProvider) emitStoreInitFailureSummary(err error, storeType string) {
	actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
		Result:       actions.SummaryResultFailure,
		Msg:          fmt.Sprintf("Failed to create %s store: %v", storeType, err),
		FailureCount: 1,
	})
}

func (s *SecretsManagerValueProvider) emitEnvStoreWarning(storeType string) {
	actions.AddSummary(githubStepName, actions.SummaryRowWithCounts{
		Result:       actions.SummaryResultWarning,
		Msg:          fmt.Sprintf("Environment specific %s do not exist for environment \"%v\"", storeType, s.ghaClaims.Environment),
		WarningCount: 1,
	})
}

func (s SecretsManagerValueProvider) repoOrEnvStoreFromSecretARNs(ctx context.Context, repoArn, envArn string) (KVStoreStringReader, error) {
	repoStore, err := s.mapStoreFromSecretARN(ctx, repoArn)
	if err != nil {
		return nil, fmt.Errorf("error retrieving repo-level values from Secrets Manager: %w", err)
	}

	envStore, err := s.mapStoreFromSecretARN(ctx, envArn)
	if err != nil {
		if isResourceNotFoundException(err) {
			envStore = &MapBackedKVStore{store: map[string]string{}}
		} else {
			return nil, fmt.Errorf("error retrieving environment-specific values from Secrets Manager: %w", err)
		}
	}

	// When the workflow is running in the context of a specific GHA environment, we expect to find
	// environment-specific values in Secrets Manager.
	// Emit a warning when no environment-specific values are stored at all.
	// If an empty environment store is retrieved from Secrets Manager, the environment is configured to
	// use repo-level values only.
	if envArn != "" && envStore.IsEmpty() && isResourceNotFoundException(err) {
		slog.Warn("no environment-specific values found in Secrets Manager, only repo-level values will be available", "environment", s.ghaClaims.Environment, "arn", envArn)
		err = envStoreNotFoundError{msg: fmt.Sprintf("no environment-specific values found in Secrets Manager for environment %s", s.ghaClaims.Environment)}
	}
	if envArn != "" && envStore.IsEmpty() && err == nil {
		slog.Info("environment-specific values are empty in Secrets Manager, only repo-level values will be available", "environment", s.ghaClaims.Environment, "arn", envArn)
	}

	return &RepoOrEnvKVStore{
		repoStore: repoStore,
		envStore:  envStore,
	}, err
}

func (s SecretsManagerValueProvider) mapStoreFromSecretARN(ctx context.Context, arn string) (KVStoreStringReader, error) {
	if arn == "" {
		return &MapBackedKVStore{store: map[string]string{}}, nil
	}
	client := secretsmanager.NewFromConfig(s.awsConfig)

	slog.Debug("Retrieving secret value from AWS Secrets Manager", "arn", arn)
	secretOutput, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(arn)})
	if err != nil {
		return nil, err
	}

	kvMap := make(map[string]string, len(s.valuesConfig))
	if err := json.Unmarshal([]byte(aws.ToString(secretOutput.SecretString)), &kvMap); err != nil {
		return nil, fmt.Errorf("error unmarshalling value from Secrets Manager: %w", err)
	}

	return &MapBackedKVStore{store: kvMap}, nil
}
