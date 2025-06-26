/*
Copyright 2025 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadStats(t *testing.T) {
	tests := []struct {
		name        string
		stats       Stats
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid stats",
			stats: Stats{
				BundleSizes: map[string]int64{
					"main.js":   100000,
					"vendor.js": 200000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js":   30000,
					"vendor.js": 60000,
				},
				FileSizes: map[string]int64{
					"app.js":    50000,
					"styles.css": 25000,
				},
				FileGzipSizes: map[string]int64{
					"app.js":    15000,
					"styles.css": 8000,
				},
				ModuleSizes: map[string]int64{
					"react":     150000,
					"react-dom": 120000,
				},
				ModuleGzipSizes: map[string]int64{
					"react":     45000,
					"react-dom": 36000,
				},
				TotalSize:     300000,
				TotalGzipSize: 90000,
			},
		},
		{
			name: "empty stats",
			stats: Stats{
				BundleSizes:     map[string]int64{},
				BundleGzipSizes: map[string]int64{},
				FileSizes:       map[string]int64{},
				FileGzipSizes:   map[string]int64{},
				ModuleSizes:     map[string]int64{},
				ModuleGzipSizes: map[string]int64{},
				TotalSize:       0,
				TotalGzipSize:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()
			statsFile := filepath.Join(tmpDir, "stats.json")

			// Write stats to file
			data, err := json.Marshal(tt.stats)
			require.NoError(t, err)
			err = os.WriteFile(statsFile, data, 0644)
			require.NoError(t, err)

			// Load and verify
			loaded, err := LoadStats(statsFile)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.stats, loaded)
			}
		})
	}
}

func TestLoadStats_Errors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string) string
		errorMsg  string
	}{
		{
			name: "file not found",
			setup: func(tmpDir string) string {
				return filepath.Join(tmpDir, "nonexistent.json")
			},
			errorMsg: "failed to read stats report",
		},
		{
			name: "invalid JSON",
			setup: func(tmpDir string) string {
				path := filepath.Join(tmpDir, "invalid.json")
				err := os.WriteFile(path, []byte("not valid json"), 0644)
				require.NoError(t, err)
				return path
			},
			errorMsg: "failed to unmarshal stats report",
		},
		{
			name: "wrong JSON structure",
			setup: func(tmpDir string) string {
				path := filepath.Join(tmpDir, "wrong.json")
				// This JSON is valid but doesn't match the Stats structure
				// It will unmarshal successfully but with empty/zero values
				err := os.WriteFile(path, []byte(`{"wrong": "structure"}`), 0644)
				require.NoError(t, err)
				return path
			},
			errorMsg: "", // Remove error expectation since it unmarshals successfully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := tt.setup(tmpDir)

			stats, err := LoadStats(path)
			if tt.errorMsg != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				// For wrong JSON structure, it should load but with empty values
				require.NoError(t, err)
				require.Empty(t, stats.BundleSizes)
				require.Empty(t, stats.ModuleSizes)
				require.Zero(t, stats.TotalSize)
			}
		})
	}
}