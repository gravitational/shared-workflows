package kvstore

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"filippo.io/age"
)

const (
	// migrationAgePublicKey is the key where the public key for age encryption is stored in the kvstore.
	migrationAgePublicKey = "kvstore_migrate_age_public"
	// migrationS3BucketKey is the key where the S3 bucket name for migration is stored in the kvstore.
	migrationS3BucketKey = "kvstore_migrate_s3_bucket"
	// ghaVarsContextEnv is the environment variable where the existing GitHub Actions variables context is stored for migration.
	ghaVarsContextEnv = "GHA_VARS_CONTEXT"
	// ghaSecretsContextEnv is the environment variable where the existing GitHub Actions secrets context is stored for migration.
	ghaSecretsContextEnv = "GHA_SECRETS_CONTEXT"
)

type MigrationConfig struct {
	AgePublicKey    string `json:"agePublicKey"`
	S3Bucket        string `json:"-"`
	ExistingVars    string `json:"vars"`
	ExistingSecrets string `json:"secrets"`
}

// GetMigrationConfig checks for the presence of migration configuration
// returns the migration configuration if found, along with a boolean indicating
// its presence.
func (p SecretsManagerValueProvider) GetMigrationConfig() (MigrationConfig, bool) {
	agePublicKey, ok := p.variables.GetValue(migrationAgePublicKey)
	//  valueProvider.GetValue(migrationAgePublicKey)
	if !ok {
		slog.Info("No age public key found in kvstore for migration, skipping migration.", "key", migrationAgePublicKey)
		return MigrationConfig{}, false
	}

	// validate that the age public key is well-formed
	recipient, err := age.ParseX25519Recipient(agePublicKey)
	if err != nil {
		slog.Error("Invalid age public key found in kvstore for migration.", "error", err)
		return MigrationConfig{}, false
	}

	s3Bucket, ok := p.variables.GetValue(migrationS3BucketKey)
	if !ok {
		slog.Info("No S3 bucket found in kvstore for migration, skipping migration.", "key", migrationS3BucketKey)
		return MigrationConfig{}, false
	}

	vars, err := ageEncrypt(os.Getenv(ghaVarsContextEnv), recipient)
	if err != nil {
		slog.Error("Failed to encrypt existing GitHub Actions variables for migration.", "error", err)
		return MigrationConfig{}, false
	}
	secrets, err := ageEncrypt(os.Getenv(ghaSecretsContextEnv), recipient)
	if err != nil {
		slog.Error("Failed to encrypt existing GitHub Actions secrets for migration.", "error", err)
		return MigrationConfig{}, false
	}

	return MigrationConfig{
		AgePublicKey:    agePublicKey,
		S3Bucket:        s3Bucket,
		ExistingVars:    vars,
		ExistingSecrets: secrets,
	}, true
}

func ageEncrypt(plainText string, recipient age.Recipient) (string, error) {
	data, err := age.EncryptReader(strings.NewReader(plainText), recipient)
	if err != nil {
		return "", fmt.Errorf("failed to provision age encryptor: %w", err)
	}
	encryptedBytes, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}
	return string(encryptedBytes), nil
}
