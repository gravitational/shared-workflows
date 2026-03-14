// Package kvstore provides functionality for retrieving secrets from AWS Secrets Manager.

package kvstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

const (
	repoARNFormat = "arn:aws:secretsmanager:%s:%s:secret:%s/repo/%s"
	envARNFormat  = "arn:aws:secretsmanager:%s:%s:secret:%s/repo/%s/env/%s"
)

type SecretsManagerValueProvider struct {
	awsConfig    aws.Config
	smConfig     config.SecretsManagerConfig
	valuesConfig []config.EnvValue
	ghaClaims    config.GHAClaims
	secrets      KVStoreStringReader
	variables    KVStoreStringReader
	envValues    []ghaEnvValue
}

type ghaEnvValue struct {
	config.EnvValue
	value string
}

func newGHAEnvValue(envValue config.EnvValue, value string) ghaEnvValue {
	return ghaEnvValue{
		EnvValue: envValue,
		value:    value,
	}
}

func (e ghaEnvValue) ghaMask() string {
	if e.ValueType == "secret" && e.value != "" {
		return fmt.Sprintf("::add-mask::%s", e.value)
	}
	return ""
}

func (e ghaEnvValue) ghaEnv() string {
	return fmt.Sprintf("%s=\"%s\"", e.EnvVar, e.value)
}

func NewSecretsManagerValueProvider(ctx context.Context, awsConfig aws.Config, smConfig config.SecretsManagerConfig, values []config.EnvValue, ghaClaims config.GHAClaims) (*SecretsManagerValueProvider, error) {
	provider := &SecretsManagerValueProvider{
		awsConfig:    awsConfig,
		smConfig:     smConfig,
		valuesConfig: values,
		ghaClaims:    ghaClaims,
	}

	repoSecretsARN := fmt.Sprintf("%s/secrets", fmt.Sprintf(repoARNFormat, smConfig.Region, smConfig.AccountID, ghaClaims.Enterprise, ghaClaims.Repository))
	repoVariablesARN := fmt.Sprintf("%s/variables", fmt.Sprintf(repoARNFormat, smConfig.Region, smConfig.AccountID, ghaClaims.Enterprise, ghaClaims.Repository))
	var envSecretsARN, envVariablesARN string
	if ghaClaims.Environment != "" {
		envSecretsARN = fmt.Sprintf("%s/secrets", fmt.Sprintf(envARNFormat, smConfig.Region, smConfig.AccountID, ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.Environment))
		envVariablesARN = fmt.Sprintf("%s/variables", fmt.Sprintf(envARNFormat, smConfig.Region, smConfig.AccountID, ghaClaims.Enterprise, ghaClaims.Repository, ghaClaims.Environment))
	}

	secrets, err := provider.repoOrEnvStoreFromSecretARNs(ctx, repoSecretsARN, envSecretsARN)
	if err != nil {
		return nil, err
	}
	provider.secrets = secrets

	variables, err := provider.repoOrEnvStoreFromSecretARNs(ctx, repoVariablesARN, envVariablesARN)
	if err != nil {
		return nil, err
	}
	provider.variables = variables

	provider.populateEnvValues()

	provider.maskAllSecretValues()

	fmt.Println("::group::Environment variables to set for subsequent steps")
	defer fmt.Println("::endgroup::")
	fmt.Print(provider.GenerateEnvLinesToAppend())

	return provider, nil
}

func (s *SecretsManagerValueProvider) populateEnvValues() {
	for _, v := range s.valuesConfig {
		var value string
		var ok bool
		if v.ValueType == "secret" {
			value, ok = s.secrets.GetValue(v.LookupKey())
			if !ok {
				slog.Warn("secret value not found in Secrets Manager for env var", "envVar", v.EnvVar, "keyOverride", v.KeyOverride)
				continue
			}
		} else if v.ValueType == "variable" {
			value, ok = s.variables.GetValue(v.LookupKey())
			if !ok {
				slog.Warn("variable value not found in Secrets Manager for env var", "envVar", v.EnvVar, "keyOverride", v.KeyOverride)
				continue
			}
		}
		s.envValues = append(s.envValues, newGHAEnvValue(v, value))
	}
}

// maskAllSecretValues iterates through the values and adds a mask for any value of type "secret" so that they are not exposed in GitHub Actions logs if printed by subsequent steps.
func (s *SecretsManagerValueProvider) maskAllSecretValues() {
	fmt.Println("::group::Masking secret values in GitHub Actions logs")
	defer fmt.Println("::endgroup::")
	for _, v := range s.envValues {
		if v.ghaMask() != "" {
			fmt.Println(v.ghaMask())
		}
	}
}

func (s *SecretsManagerValueProvider) GenerateEnvLinesToAppend() string {
	var sb strings.Builder
	for _, v := range s.envValues {
		sb.WriteString(v.ghaEnv())
		sb.WriteString("\n")
	}
	return sb.String()
}

func (s SecretsManagerValueProvider) repoOrEnvStoreFromSecretARNs(ctx context.Context, repoArn, envArn string) (KVStoreStringReader, error) {
	repoStore, err := s.mapStoreFromSecretARN(ctx, repoArn)
	if err != nil {
		return nil, fmt.Errorf("error retrieving repo-level values from Secrets Manager: %w", err)
	}

	envStore, err := s.mapStoreFromSecretARN(ctx, envArn)
	if err != nil {
		slog.Warn("failed to retrieve environment-specific values from Secrets Manager, only repo-level values will be available", "error", err)
		envStore = &MapBackedKVStore{store: map[string]string{}}
	}

	return &RepoOrEnvKVStore{
		repoStore: repoStore,
		envStore:  envStore,
	}, nil
}

func (s SecretsManagerValueProvider) mapStoreFromSecretARN(ctx context.Context, arn string) (KVStoreStringReader, error) {
	if arn == "" {
		return &MapBackedKVStore{store: map[string]string{}}, nil
	}
	client := secretsmanager.NewFromConfig(s.awsConfig)

	secretOutput, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(arn)})
	if err != nil {
		return nil, err
	}

	kvMap := make(map[string]string)
	if err := json.Unmarshal([]byte(aws.ToString(secretOutput.SecretString)), &kvMap); err != nil {
		return nil, fmt.Errorf("error unmarshalling value from Secrets Manager: %w", err)
	}

	return &MapBackedKVStore{store: kvMap}, nil
}

type KVStoreStringReader interface {
	GetValue(key string) (string, bool)
}

type RepoOrEnvKVStore struct {
	repoStore KVStoreStringReader
	envStore  KVStoreStringReader
}

func (s *RepoOrEnvKVStore) GetValue(key string) (string, bool) {
	// first check environment-specific store, then repo-level store
	envValue, ok := s.envStore.GetValue(key)
	if ok {
		return envValue, true
	}
	return s.repoStore.GetValue(key)
}

type MapBackedKVStore struct {
	store map[string]string
}

func (m *MapBackedKVStore) GetValue(key string) (string, bool) {
	value, ok := m.store[key]
	if !ok {
		return "", false
	}
	return value, true
}
