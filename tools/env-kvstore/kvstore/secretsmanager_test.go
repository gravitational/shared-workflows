package kvstore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// mockSecretsManagerClient is a test double for SecretsManagerClient.
// supports pre-configured responses keyed by ARN
type mockSecretsManagerClient struct {
	// secrets is a map of successful responses by ARN
	secrets map[string]string
	// apiErrors is a map of error responses by ARN
	apiErrors map[string]error
}

func (m *mockSecretsManagerClient) GetSecretValue(_ context.Context, params *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	arn := aws.ToString(params.SecretId)
	if err, ok := m.apiErrors[arn]; ok {
		return nil, err
	}
	secret, ok := m.secrets[arn]
	if !ok {
		return nil, fmt.Errorf("no mock response configured for ARN %q", arn)
	}
	return &secretsmanager.GetSecretValueOutput{
		SecretString: aws.String(secret),
	}, nil
}

func newTestProvider(client SecretsManagerClient) *SecretsManagerValueProvider {
	return &SecretsManagerValueProvider{
		smClient: client,
		smConfig: config.SecretsManagerConfig{
			Region:    "us-east-1",
			AccountID: "123456789012",
		},
		ghaClaims: config.GHAClaims{
			Enterprise:  "myenterprise",
			Repository:  "myorg/myrepo",
			Environment: "staging",
		},
		valuesConfig: []config.EnvValue{
			{EnvVar: "MY_SECRET", ValueType: "secret"},
			{EnvVar: "MY_VAR", ValueType: "variable"},
		},
	}
}

func TestMapStoreFromSecretARN_EmptyARN(t *testing.T) {
	p := newTestProvider(&mockSecretsManagerClient{})
	store, err := p.mapStoreFromSecretARN(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error for empty ARN, got %v", err)
	}
	if !store.IsEmpty() {
		t.Error("expected empty store for empty ARN")
	}
}

func TestMapStoreFromSecretARN_Success(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/secrets"
	client := &mockSecretsManagerClient{
		secrets: map[string]string{
			arn: `{"MY_SECRET":"supersecret","OTHER":"value"}`,
		},
	}

	p := newTestProvider(client)
	store, err := p.mapStoreFromSecretARN(context.Background(), arn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := store.GetValue("MY_SECRET")
	if !ok || val != "supersecret" {
		t.Errorf("GetValue(MY_SECRET) = (%q, %v), want (supersecret, true)", val, ok)
	}
	val, ok = store.GetValue("OTHER")
	if !ok || val != "value" {
		t.Errorf("GetValue(OTHER) = (%q, %v), want (value, true)", val, ok)
	}
}

func TestMapStoreFromSecretARN_APIError(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/secrets"
	apiErr := errors.New("ResourceNotFoundException: secret not found")
	client := &mockSecretsManagerClient{
		apiErrors: map[string]error{arn: apiErr},
	}

	p := newTestProvider(client)
	_, err := p.mapStoreFromSecretARN(context.Background(), arn)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, apiErr) {
		t.Errorf("expected wrapped apiErr, got %v", err)
	}
}

func TestMapStoreFromSecretARN_InvalidJSON(t *testing.T) {
	const arn = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/secrets"
	client := &mockSecretsManagerClient{
		secrets: map[string]string{
			arn: `not-valid-json`,
		},
	}

	p := newTestProvider(client)
	_, err := p.mapStoreFromSecretARN(context.Background(), arn)
	if err == nil {
		t.Fatal("expected an unmarshalError, got nil")
	}
	var unmarshal unmarshalError
	if !errors.As(err, &unmarshal) {
		t.Errorf("expected unmarshalError, got %T: %v", err, err)
	}
}

const (
	testRepoSecretsARN   = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/secrets"
	testRepoVariablesARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/variables"
	testEnvSecretsARN    = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/env/staging/secrets"
	testEnvVariablesARN  = "arn:aws:secretsmanager:us-east-1:123456789012:secret:myenterprise/repo/myorg/myrepo/env/staging/variables"
)

func TestInitializeStores_Success(t *testing.T) {
	client := &mockSecretsManagerClient{
		secrets: map[string]string{
			testRepoSecretsARN:   `{"REPO_SECRET":"repo_val"}`,
			testRepoVariablesARN: `{"REPO_VAR":"repo_var_val"}`,
			testEnvSecretsARN:    `{"REPO_SECRET":"env_override","ENV_SECRET":"env_only"}`,
			testEnvVariablesARN:  `{"ENV_VAR":"env_var_val"}`,
		},
	}

	p := newTestProvider(client)
	if err := p.initializeStores(context.Background()); err != nil {
		t.Fatalf("initializeStores() error = %v", err)
	}

	// env-specific value should shadow repo-level value
	if val, ok := p.secrets.GetValue("REPO_SECRET"); !ok || val != "env_override" {
		t.Errorf("secrets.GetValue(REPO_SECRET) = (%q, %v), want (env_override, true)", val, ok)
	}
	if val, ok := p.secrets.GetValue("ENV_SECRET"); !ok || val != "env_only" {
		t.Errorf("secrets.GetValue(ENV_SECRET) = (%q, %v), want (env_only, true)", val, ok)
	}
	if val, ok := p.variables.GetValue("REPO_VAR"); !ok || val != "repo_var_val" {
		t.Errorf("variables.GetValue(REPO_VAR) = (%q, %v), want (repo_var_val, true)", val, ok)
	}
	if val, ok := p.variables.GetValue("ENV_VAR"); !ok || val != "env_var_val" {
		t.Errorf("variables.GetValue(ENV_VAR) = (%q, %v), want (env_var_val, true)", val, ok)
	}
}

func TestInitializeStores_EnvStoreFetchFails(t *testing.T) {
	// env ARNs fail; repo ARNs succeed. initializeStores should succeed with repo-only data.
	apiErr := errors.New("AccessDeniedException")
	client := &mockSecretsManagerClient{
		secrets: map[string]string{
			testRepoSecretsARN:   `{"REPO_SECRET":"repo_val"}`,
			testRepoVariablesARN: `{"REPO_VAR":"repo_var_val"}`,
		},
		apiErrors: map[string]error{
			testEnvSecretsARN:   apiErr,
			testEnvVariablesARN: apiErr,
		},
	}

	p := newTestProvider(client)
	if err := p.initializeStores(context.Background()); err != nil {
		t.Fatalf("initializeStores() should succeed when only env store is unavailable, got %v", err)
	}

	if val, ok := p.secrets.GetValue("REPO_SECRET"); !ok || val != "repo_val" {
		t.Errorf("secrets.GetValue(REPO_SECRET) = (%q, %v), want (repo_val, true)", val, ok)
	}
	if val, ok := p.variables.GetValue("REPO_VAR"); !ok || val != "repo_var_val" {
		t.Errorf("variables.GetValue(REPO_VAR) = (%q, %v), want (repo_var_val, true)", val, ok)
	}
}

func TestInitializeStores_RepoStoreFetchFails(t *testing.T) {
	// repo secrets ARN fails — this is a non-recoverable error.
	client := &mockSecretsManagerClient{
		apiErrors: map[string]error{
			testRepoSecretsARN: errors.New("ResourceNotFoundException"),
		},
	}

	p := newTestProvider(client)
	if err := p.initializeStores(context.Background()); err == nil {
		t.Fatal("initializeStores() should fail when repo store is unavailable, got nil")
	}
}
