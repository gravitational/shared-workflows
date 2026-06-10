package kvstore

import (
	"io"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
)

func TestGetMigrationConfig(t *testing.T) {
	tests := []struct {
		name             string
		recipientFactory func(t *testing.T) (recipient string, identity age.Identity)
	}{
		{
			name: "x25519 recipient",
			recipientFactory: func(t *testing.T) (string, age.Identity) {
				t.Helper()

				identity, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatalf("GenerateX25519Identity() error = %v", err)
				}

				return identity.Recipient().String(), identity
			},
		},
		{
			name: "hybrid recipient",
			recipientFactory: func(t *testing.T) (string, age.Identity) {
				t.Helper()

				identity, err := age.GenerateHybridIdentity()
				if err != nil {
					t.Fatalf("GenerateHybridIdentity() error = %v", err)
				}

				return identity.Recipient().String(), identity
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recipient, identity := tt.recipientFactory(t)

			t.Setenv(ghaVarsContextEnv, `{"var":"value"}`)
			t.Setenv(ghaSecretsContextEnv, `{"secret":"value"}`)

			provider := SecretsManagerValueProvider{
				variables: &MapBackedKVStore{
					store: map[string]string{
						migrationAgePublicKey: recipient,
						migrationS3BucketKey:  "migration-bucket",
					},
				},
			}

			got, err := provider.GetMigrationConfig()
			if err != nil {
				t.Fatalf("GetMigrationConfig() error = %v", err)
			}

			if got.AgePublicKey != recipient {
				t.Fatalf("AgePublicKey = %q, want %q", got.AgePublicKey, recipient)
			}
			if got.S3Bucket != "migration-bucket" {
				t.Fatalf("S3Bucket = %q, want %q", got.S3Bucket, "migration-bucket")
			}

			if decrypted := decryptAgePayload(t, got.EncryptedVars, identity); decrypted != `{"var":"value"}` {
				t.Fatalf("ExistingVars decrypted to %q", decrypted)
			}
			if decrypted := decryptAgePayload(t, got.EncryptedSecrets, identity); decrypted != `{"secret":"value"}` {
				t.Fatalf("ExistingSecrets decrypted to %q", decrypted)
			}
		})
	}
}

func decryptAgePayload(t *testing.T, encrypted string, identity age.Identity) string {
	t.Helper()

	reader, err := age.Decrypt(armor.NewReader(strings.NewReader(encrypted)), identity)
	if err != nil {
		t.Fatalf("age.Decrypt() error = %v", err)
	}

	plainText, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll() error = %v", err)
	}

	return string(plainText)
}
