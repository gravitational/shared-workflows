package main

import (
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-kvstore/config"
)

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
