package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the configuration values for the application.
type Config struct {
	Cognito        CognitoConfig
	SecretsManager SecretsManagerConfig
	Values         ValuesConfig
	GHA            GHAConfig
}

// CognitoConfig holds the necessary information for performing Cognito authentication.
type CognitoConfig struct {
	AccountID      string
	IdentityPoolID string
	RoleARN        string
}

type SecretsManagerConfig struct {
	AccountID string
	Region    string
}

type GHAConfig struct {
	IDTokenRequestToken string
	IDTokenRequestURL   string
	EnterpriseName      string
}

type ValuesConfig struct {
	ValuesInput string
	Items       []EnvValue
}

// EnvValue represents a value to be retrieved from the KVStore and set as an
// environment variable in GitHub Actions.
type EnvValue struct {
	// EnvVar is the name of the environment variable to set in the GitHub Actions workflow.
	EnvVar string
	// ValueType indicates whether the value is a variable or a secret. Determines retrieval
	// namespace and whether value is masked in GitHub Actions logs.
	ValueType string
	// KeyOverride is used to lookup the value in AWS Secrets Manager by a different key than 
	// the environment variable name. Use when a branch-specific value is needed.
	KeyOverride string
}

func NewFromEnv() Config {
	secretsManagerAccountID := os.Getenv("INPUT_SECRETS-MANAGER-ACCOUNT-ID")
	secretsManagerRegion := os.Getenv("INPUT_SECRETS-MANAGER-REGION")
	cognitoAccountID := os.Getenv("INPUT_COGNITO-ACCOUNT-ID")
	cognitoIdentityPoolID := os.Getenv("INPUT_COGNITO-IDENTITY-POOL-ID")
	cognitoRoleARN := os.Getenv("INPUT_COGNITO-ROLE-ARN")
	values := os.Getenv("INPUT_VALUES")
	ghaIDTokenRequestToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	ghaIDTokenRequestURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")

	return Config{
		Cognito: CognitoConfig{
			AccountID:      cognitoAccountID,
			IdentityPoolID: cognitoIdentityPoolID,
			RoleARN:        cognitoRoleARN,
		},
		SecretsManager: SecretsManagerConfig{
			AccountID: secretsManagerAccountID,
			Region:    secretsManagerRegion,
		},
		Values: ValuesConfig{
			ValuesInput: values,
		},
		GHA: GHAConfig{
			IDTokenRequestToken: ghaIDTokenRequestToken,
			IDTokenRequestURL:   ghaIDTokenRequestURL,
		},
	}
}

// Validate checks that all required configuration fields are set and that the Values field is properly formatted.
// It also fills in any missing AccountID or Region fields from the Cognito Role ARN or Identity Pool ID.
func (c *Config) Validate() error {
	// AccountID can be derived from the Role ARN
	accountFromRoleARN := ""
	if c.Cognito.RoleARN != "" {
		parts := strings.Split(c.Cognito.RoleARN, ":")
		if len(parts) >= 5 {
			accountFromRoleARN = parts[4]
		}
	}
	regionFromIdentityPoolID := c.Cognito.GetRegion()

	if c.Cognito.IdentityPoolID == "" {
		return fmt.Errorf("missing required config: Cognito Identity Pool ID")
	}
	if c.Cognito.RoleARN == "" {
		return fmt.Errorf("missing required config: Cognito Role ARN")
	}
	if c.Cognito.AccountID == "" {
		if accountFromRoleARN == "" {
			return fmt.Errorf("missing required config: Cognito Account ID")
		}
		c.Cognito.AccountID = accountFromRoleARN
	}

	if c.GHA.IDTokenRequestToken == "" || c.GHA.IDTokenRequestURL == "" {
		return fmt.Errorf("missing required config: GitHub Actions ID token request token and URL")
	}

	if c.SecretsManager.Region == "" {
		if regionFromIdentityPoolID == "" {
			return fmt.Errorf("missing required config: AWS region where Secrets Manager secrets are located")
		}
		c.SecretsManager.Region = regionFromIdentityPoolID
	}
	if c.SecretsManager.AccountID == "" {
		if accountFromRoleARN == "" {
			return fmt.Errorf("missing required config: AWS account ID where Secrets Manager secrets are located")
		}
		c.SecretsManager.AccountID = accountFromRoleARN
	}

	if c.Values.ValuesInput == "" {
		return fmt.Errorf("at least one value must be specified to retrieve from KVStore")
	}

	if err := c.Values.ParseValues(); err != nil {
		return fmt.Errorf("cannot parse values: %v", err)
	}

	return nil
}

// ParseValues parses the Values string into a slice of ValueConfig structs.
// Each line is comma-separated. The expected format for each line is:
// ENV_VAR_NAME,VALUE_TYPE,NAME_OVERRIDE
// Lines can be separated by newlines or semicolons.
func (c *ValuesConfig) ParseValues() error {
	valueConfigs := []EnvValue{}

	valueEntries := strings.Split(c.ValuesInput, "\n")

	for _, entry := range valueEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ",")
		if len(parts) < 2 || len(parts) > 3 {
			return fmt.Errorf("invalid value entry: \"%s\". Expected format: ENV_VAR_NAME(req),VALUE_TYPE(req),NAME_OVERRIDE(optional)", entry)
		}

		valueConfig := EnvValue{
			EnvVar:    strings.TrimSpace(parts[0]),
			ValueType: strings.ToLower(strings.TrimSpace(parts[1])),
		}

		if valueConfig.ValueType != "variable" && valueConfig.ValueType != "secret" {
			return fmt.Errorf("invalid value type for entry: \"%s\". Expected 'variable' or 'secret'", entry)
		}

		if len(parts) >= 3 {
			valueConfig.KeyOverride = strings.TrimSpace(parts[2])
		}

		valueConfigs = append(valueConfigs, valueConfig)
	}

	c.Items = valueConfigs
	return nil
}

// GetRegion returns the AWS region of the Cognito Identity Pool.
func (c *CognitoConfig) GetRegion() string {
	// Region can be derived from the Identity Pool ID in the format REGION:UUID
	if c.IdentityPoolID != "" {
		region, _, ok := strings.Cut(c.IdentityPoolID, ":")
		if ok {
			return region
		}
	}
	return ""
}
