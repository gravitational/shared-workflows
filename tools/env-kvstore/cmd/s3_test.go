package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"
	"github.com/gravitational/shared-workflows/tools/env-kvstore/kvstore"
)

type stubMigrationConfigGetter struct {
	config    kvstore.MigrationConfig
	err       error
	hasConfig bool
}

func (s stubMigrationConfigGetter) GetMigrationConfig() (kvstore.MigrationConfig, error) {
	return s.config, s.err
}

func (s stubMigrationConfigGetter) HasMigrationConfig() bool {
	return s.hasConfig
}

func TestGenerateS3Key(t *testing.T) {
	tests := []struct {
		name      string
		claims    config.GHAClaims
		wantKey   string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "builds key with environment",
			claims: config.GHAClaims{
				Enterprise:  "teleport",
				Repository:  "gravitational/shared-workflows",
				Workflow:    "deploy",
				Environment: "prod",
				RunID:       "12345",
			},
			wantKey: "teleport/gravitational/shared-workflows/deploy/prod/12345.json",
		},
		{
			name: "uses fallback when environment is empty",
			claims: config.GHAClaims{
				Enterprise: "teleport",
				Repository: "gravitational/shared-workflows",
				Workflow:   "deploy",
				RunID:      "12345",
			},
			wantKey: "teleport/gravitational/shared-workflows/deploy/__noenv__/12345.json",
		},
		{
			name: "fails when required claims are missing",
			claims: config.GHAClaims{
				Enterprise: "teleport",
				Workflow:   "deploy",
				RunID:      "12345",
			},
			wantErr:   true,
			errSubstr: "missing required GHA claims",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, err := generateS3Key(tt.claims)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("generateS3Key() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("generateS3Key() error = %q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("generateS3Key() error = %v, want nil", err)
			}
			if gotKey != tt.wantKey {
				t.Fatalf("generateS3Key() = %q, want %q", gotKey, tt.wantKey)
			}
		})
	}
}

func TestMigrationUploaderUpload(t *testing.T) {
	t.Run("skip error is non-fatal", func(t *testing.T) {
		uploader := migrationUploader{
			migrationConfig: stubMigrationConfigGetter{
				err:       kvstore.NewSkipMigrationError("skip upload"),
				hasConfig: true,
			},
		}

		if err := uploader.Upload(context.Background()); err != nil {
			t.Fatalf("Upload() error = %v, want nil", err)
		}
	})

	t.Run("generic migration config error is returned", func(t *testing.T) {
		uploader := migrationUploader{
			migrationConfig: stubMigrationConfigGetter{
				err:       fmt.Errorf("boom"),
				hasConfig: true,
			},
		}

		err := uploader.Upload(context.Background())
		if err == nil {
			t.Fatal("Upload() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "error getting migration configuration") {
			t.Fatalf("Upload() error = %q, want wrapped migration configuration error", err.Error())
		}
	})
}
