package config

import (
	"testing"
)

func TestConfigValidation(t *testing.T) {
	validGHAConfig := GHAConfig{
		IDTokenRequestToken: "example-token",
		IDTokenRequestURL:   "http://test.github.com/token",
	}
	validCognitoConfig := CognitoConfig{
		IdentityPoolID: "us-west-2:example-pool-id",
		RoleARN:        "arn:aws:iam::123456789012:role/example-role",
	}
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "full config with all fields set",
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
				GHA:    validGHAConfig,
				Values: "MY_VAR,variable,my-var",
			},
			wantErr: false,
		},
		{
			name: "valid minimal config with Cognito auth and inferrable AccountID and Region",
			config: Config{
				Cognito: validCognitoConfig,
				GHA:     validGHAConfig,
				Values:  "MY_VAR,variable,my-var",
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
				GHA:    validGHAConfig,
				Values: "MY_VAR,secret,my-secret",
			},
			wantErr: true,
		},
		{
			name: "missing values",
			config: Config{
				Cognito: validCognitoConfig,
				GHA:     validGHAConfig,
			},
			wantErr: true,
		},
		{
			name: "invalid values format",
			config: Config{
				Cognito: validCognitoConfig,
				GHA:     validGHAConfig,
				Values:  "INVALID_FORMAT_TOO_FEW_COLUMNS",
			},
			wantErr: true,
		},
		{
			name: "invalid value type",
			config: Config{
				Cognito: validCognitoConfig,
				GHA:     validGHAConfig,
				Values:  "MY_VAR,not_a_type,repo,my-secret",
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
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "example-token")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://test.github.com/token")
	t.Setenv("INPUT_COGNITO-IDENTITY-POOL-ID", "us-west-2:example-pool-id")
	t.Setenv("INPUT_COGNITO-ROLE-ARN", "arn:aws:iam::123456789012:role/example-role")
	t.Setenv("INPUT_VALUES", "MY_VAR,variable,my-var\nANOTHER_VAR,secret,env")
	cfg := NewFromEnv()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	if cfg.SecretsManager.Region != "us-west-2" {
		t.Errorf("Expected SecretsManager Region to be 'us-west-2', got '%s'", cfg.SecretsManager.Region)
	}
	if cfg.SecretsManager.AccountID != "123456789012" {
		t.Errorf("Expected SecretsManager AccountID to be '123456789012', got '%s'", cfg.SecretsManager.AccountID)
	}
	if cfg.Cognito.AccountID != "123456789012" {
		t.Errorf("Expected Cognito AccountID to be '123456789012', got '%s'", cfg.Cognito.AccountID)
	}
}

func TestParseValues(t *testing.T) {
	cfg := Config{
		Values: "\n\n\nMY_VAR,variable,my-var\nANOTHER_VAR,secret,env",
	}
	values, err := cfg.ParseValues()
	if err != nil {
		t.Fatalf("ParseValues() error = %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
}
