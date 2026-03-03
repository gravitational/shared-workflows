package config

import (
	"fmt"
	"os"
	"strings"
)

type envGetter interface {
	getEnv(key string) string
}

type envGetterOS struct{}

func (e *envGetterOS) getEnv(key string) string {
	return os.Getenv(key)
}

// Config holds the configuration values for the application.
type Config struct {
	Cognito        CognitoConfig
	SecretsManager SecretsManagerConfig
	Values         string
	GHA            GHAConfig
	envGetter
}

func newConfig(envGetter envGetter) *Config {
	cfg := &Config{envGetter: envGetter}
	return cfg
}

func New() *Config {
	return newConfig(&envGetterOS{})
}

// CognitoConfig holds the necessary information for performing Cognito authentication.
type CognitoConfig struct {
	AccountID      string
	IdentityPoolID string
	Region         string
	RoleARN        string
}

type AWSSessionCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type SecretsManagerConfig struct {
	AccountID string
	Region    string
}

type GHAConfig struct {
	IDTokenRequestToken string
	IDTokenRequestURL   string
	GitHubToken         string
	Environment         string
}

type ValueConfig struct {
	// EnvVar is the name of the environment variable to set in the GitHub Actions workflow.
	EnvVar string
	// ValueType indicates whether the value is a variable or a secret. Determines retrieval
	// namespace and whether value is masked in GitHub Actions logs.
	ValueType string
	// NameOverride allows users to specify a different name to lookup in the AWS Secrets Manager
	// secret instead of the environment variable name. This is useful when a branch-specific value
	// needs to be maintained.
	NameOverride string
}

// NewWithEnv creates a new Config and populates fields with environment variables.
func NewWithEnv() *Config {
	c := New()
	c.GetDefaultsFromEnv()
	return c
}

func (c *Config) GetDefaultsFromEnv() {
	if c.Cognito.AccountID == "" {
		c.Cognito.AccountID = c.getEnv("AWS_ACCOUNT_ID")
	}

	if c.SecretsManager.AccountID == "" {
		c.SecretsManager.AccountID = c.getEnv("AWS_ACCOUNT_ID")
	}
	if c.SecretsManager.Region == "" {
		c.SecretsManager.Region = c.getEnv("AWS_REGION")
	}

	if c.GHA.Environment == "" {
		c.GHA.Environment = c.getEnv("GITHUB_ENVIRONMENT")
	}
	if c.GHA.GitHubToken == "" {
		c.GHA.GitHubToken = c.getEnv("GITHUB_TOKEN")
	}
}

func (c *Config) Validate() error {
	if c.Cognito.IdentityPoolID == "" {
		return fmt.Errorf("missing required config: Cognito Identity Pool ID")
	}
	if c.Cognito.RoleARN == "" {
		return fmt.Errorf("missing required config: Cognito Role ARN")
	}
	if c.Cognito.AccountID == "" {
		return fmt.Errorf("missing required config: Cognito Account ID")
	}

	if c.GHA.GitHubToken == "" && (c.GHA.IDTokenRequestToken == "" || c.GHA.IDTokenRequestURL == "") {
		return fmt.Errorf("missing required config: GitHub Actions ID token request token and URL")
	}

	if c.SecretsManager.Region == "" {
		return fmt.Errorf("missing required config: AWS region where Secrets Manager secrets are located")
	}

	if c.SecretsManager.AccountID == "" {
		return fmt.Errorf("missing required config: AWS account ID where Secrets Manager secrets are located")
	}

	if c.Values == "" {
		return fmt.Errorf("at least one value must be specified to retrieve from KVStore")
	}

	if _, err := c.ParseValues(); err != nil {
		return fmt.Errorf("cannot parse values: %v", err)
	}

	return nil
}

// ParseValues parses the Values string into a slice of ValueConfig structs.
// Each line is comma-separated. The expected format for each line is:
// ENV_VAR_NAME,VALUE_TYPE,NAME_OVERRIDE
// Lines can be separated by newlines or semicolons.
func (c *Config) ParseValues() ([]ValueConfig, error) {
	valueConfigs := []ValueConfig{}

	valueEntries := strings.Split(c.Values, "\n")
	if len(valueEntries) == 1 {
		valueEntries = strings.Split(c.Values, ";")
	}

	for _, entry := range valueEntries {
		parts := strings.Split(entry, ",")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("invalid value entry: \"%s\". Expected format: ENV_VAR_NAME(req),VALUE_TYPE(req),NAME_OVERRIDE(optional)", entry)
		}

		valueConfig := ValueConfig{
			EnvVar:    strings.TrimSpace(parts[0]),
			ValueType: strings.ToLower(strings.TrimSpace(parts[1])),
		}

		if valueConfig.ValueType != "variable" && valueConfig.ValueType != "secret" {
			return nil, fmt.Errorf("invalid value type for entry: \"%s\". Expected 'variable' or 'secret'", entry)
		}

		if len(parts) >= 3 {
			valueConfig.NameOverride = strings.TrimSpace(parts[2])
		}

		valueConfigs = append(valueConfigs, valueConfig)
	}

	return valueConfigs, nil
}

// Merge merges non-empty fields from another Config into the current Config.
func (c *Config) Merge(other *Config) {
	if other.Cognito.AccountID != "" {
		c.Cognito.AccountID = other.Cognito.AccountID
	}
	if other.Cognito.IdentityPoolID != "" {
		c.Cognito.IdentityPoolID = other.Cognito.IdentityPoolID
	}
	if other.Cognito.Region != "" {
		c.Cognito.Region = other.Cognito.Region
	}
	if other.Cognito.RoleARN != "" {
		c.Cognito.RoleARN = other.Cognito.RoleARN
	}
	if other.GHA.Environment != "" {
		c.GHA.Environment = other.GHA.Environment
	}
	if other.GHA.IDTokenRequestToken != "" {
		c.GHA.IDTokenRequestToken = other.GHA.IDTokenRequestToken
	}
	if other.GHA.IDTokenRequestURL != "" {
		c.GHA.IDTokenRequestURL = other.GHA.IDTokenRequestURL
	}
	if other.GHA.GitHubToken != "" {
		c.GHA.GitHubToken = other.GHA.GitHubToken
	}
	if other.SecretsManager.AccountID != "" {
		c.SecretsManager.AccountID = other.SecretsManager.AccountID
	}
	if other.SecretsManager.Region != "" {
		c.SecretsManager.Region = other.SecretsManager.Region
	}
	if other.Values != "" {
		c.Values = other.Values
	}
}
