package kvstore

import (
	"fmt"
	"io"
	"os"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
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
	AgePublicKey     string `json:"agePublicKey"`
	S3Bucket         string `json:"-"`
	EncryptedVars    string `json:"vars"`
	EncryptedSecrets string `json:"secrets"`
}

type SkipMigrationError struct {
	msg string
}

func (e SkipMigrationError) Error() string {
	return e.msg
}

func NewSkipMigrationError(msg string) error {
	return SkipMigrationError{msg: msg}
}

// HasMigrationConfig checks if the migration configuration is present in the kvstore.
func (p SecretsManagerValueProvider) HasMigrationConfig() bool {
	_, hasPublicKey := p.variables.GetValue(migrationAgePublicKey)
	_, hasS3Bucket := p.variables.GetValue(migrationS3BucketKey)
	return hasPublicKey && hasS3Bucket
}

// GetMigrationConfig retrieves the migration configuration from kvstore and encrypts the provided GHA vars/secrets contexts.
// Returns a SkipMigrationError when migration should be skipped (e.g. no contexts were provided).
// Otherwise returns the migration configuration or an error.
func (p SecretsManagerValueProvider) GetMigrationConfig() (MigrationConfig, error) {
	s3Bucket, ok := p.variables.GetValue(migrationS3BucketKey)
	if !ok {
		return MigrationConfig{}, fmt.Errorf("no S3 bucket found in kvstore for migration")
	}

	agePublicKey, ok := p.variables.GetValue(migrationAgePublicKey)
	if !ok {
		return MigrationConfig{}, fmt.Errorf("no age public key found in kvstore for migration")
	}

	// Validate that the age recipient is well-formed. ParseRecipients supports
	// both classic X25519 and post-quantum hybrid recipients.
	recipients, err := age.ParseRecipients(strings.NewReader(agePublicKey))
	if err != nil {
		return MigrationConfig{}, fmt.Errorf("invalid age recipient found in kvstore for migration: %w", err)
	}

	ghaVarsContext := os.Getenv(ghaVarsContextEnv)
	ghaSecretsContext := os.Getenv(ghaSecretsContextEnv)
	if ghaVarsContext == "{}" && ghaSecretsContext == "{}" {
		return MigrationConfig{}, NewSkipMigrationError(
			fmt.Sprintf("missing GitHub Actions context for migration: ensure %s and %s environment variables are set", ghaVarsContextEnv, ghaSecretsContextEnv),
		)
	}

	vars, err := ageEncrypt(ghaVarsContext, recipients)
	if err != nil {
		return MigrationConfig{}, fmt.Errorf("failed to encrypt existing GitHub Actions variables for migration: %w", err)
	}
	secrets, err := ageEncrypt(ghaSecretsContext, recipients)
	if err != nil {
		return MigrationConfig{}, fmt.Errorf("failed to encrypt existing GitHub Actions secrets for migration: %w", err)
	}

	return MigrationConfig{
		AgePublicKey:     agePublicKey,
		S3Bucket:         s3Bucket,
		EncryptedVars:    vars,
		EncryptedSecrets: secrets,
	}, nil
}

func ageEncrypt(plainText string, recipients []age.Recipient) (string, error) {
	var data strings.Builder
	armorWriter := armor.NewWriter(&data)

	w, err := age.Encrypt(armorWriter, recipients...)
	if err != nil {
		return "", fmt.Errorf("failed to provision age encryptor: %w", err)
	}
	if _, err := io.WriteString(w, plainText); err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to close encryptor: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to close ascii encoder: %w", err)
	}

	return data.String(), nil
}
