package config

import (
	"testing"
)

type envGetterTest struct {
	EnvMap map[string]string
}

func (e *envGetterTest) getEnv(key string) string {
	if val, ok := e.EnvMap[key]; ok {
		return val
	}
	return ""
}

func NewTestConfig(envMap map[string]string) *Config {
	return newConfig(&envGetterTest{EnvMap: envMap})
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config with Cognito auth",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					AccountID: "123456789012",
					Region:    "us-west-2",
				},
				GHA: GHAConfig{
					IDTokenRequestToken: "example-token",
					IDTokenRequestURL:   "http://test.github.com/token",
				},
				Values: "MY_VAR,variable,my-var",
			},
			wantErr: false,
		},
		{
			name: "missing Cognito auth info",
			config: Config{
				SecretsManager: SecretsManagerConfig{
					Region:    "us-west-2",
					AccountID: "123456789012",
				},
				Values: "MY_VAR,secret,my-secret",
			},
			wantErr: true,
		},
		{
			name: "missing AWS region",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					AccountID: "123456789012",
				},
			},
			wantErr: true,
		},
		{
			name: "missing AWS account ID",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					Region: "us-west-2",
				},
			},
			wantErr: true,
		},
		{
			name: "missing values",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					Region:    "us-west-2",
					AccountID: "123456789012",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid values format",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					Region:    "us-west-2",
					AccountID: "123456789012",
				},
				Values: "INVALID_FORMAT_TOO_FEW_COLUMNS",
			},
			wantErr: true,
		},
		{
			name: "invalid value type",
			config: Config{
				Cognito: CognitoConfig{
					IdentityPoolID: "us-west-2:example-pool-id",
					AccountID:      "123456789012",
					RoleARN:        "arn:aws:iam::123456789012:role/example-role",
				},
				SecretsManager: SecretsManagerConfig{
					Region:    "us-west-2",
					AccountID: "123456789012",
				},
				Values: "MY_VAR,not_a_type,repo,my-secret",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	cfg := newConfig(&envGetterTest{EnvMap: map[string]string{
		"AWS_REGION":     "us-west-2",
		"AWS_ACCOUNT_ID": "123456789012",
		"INPUT_VALUES":   "MY_VAR,variable,my-var\nANOTHER_VAR,secret,env",
	}})
	cfg.GetDefaultsFromEnv()

	if cfg.SecretsManager.Region != "us-west-2" {
		t.Errorf("Expected SecretsManager Region to be 'us-west-2', got '%s'", cfg.SecretsManager.Region)
	}
	if cfg.SecretsManager.AccountID != "123456789012" {
		t.Errorf("Expected SecretsManager AccountID to be '123456789012', got '%s'", cfg.SecretsManager.AccountID)
	}
	if cfg.Cognito.Region != "" {
		t.Errorf("Expected Cognito Region to be empty (infer from IdentityPoolID during token exchange), got '%s'", cfg.Cognito.Region)
	}
	if cfg.Cognito.AccountID != "123456789012" {
		t.Errorf("Expected Cognito AccountID to be '123456789012', got '%s'", cfg.Cognito.AccountID)
	}
}
