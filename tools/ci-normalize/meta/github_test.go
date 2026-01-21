// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package meta

import (
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFromGithubEnv(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		wantErr   bool
		assertion func(t *testing.T, meta *record.Meta)
	}{
		{
			name: "required envs happy path",
			env: map[string]string{
				"GITHUB_REPOSITORY":  "Example/Repo",
				"GITHUB_WORKFLOW":    "CI",
				"GITHUB_JOB":         "Test",
				"GITHUB_RUN_ID":      "123456",
				"GITHUB_RUN_NUMBER":  "3",
				"GITHUB_RUN_ATTEMPT": "2",
				"GITHUB_SHA":         "ABCDEF123456",
			},
			assertion: func(t *testing.T, meta *record.Meta) {
				t.Helper()

				assert.Equal(t, record.RecordSchemaVersion, meta.RecordSchemaVersion)
				assert.NotEmpty(t, meta.ID)

				assert.Equal(t, "github.com", meta.Provider)
				assert.Equal(t, "example/repo", meta.RepositoryName)
				assert.Equal(t, "ci", meta.Workflow)
				assert.Equal(t, "test", meta.Job)
				assert.Equal(t, "abcdef123456", meta.SHA)

				ts, err := time.Parse(time.RFC3339, meta.Timestamp)
				require.NoError(t, err)
				assert.False(t, ts.After(time.Now().Add(1*time.Second)))
			},
		},
		{
			name: "missing required env",
			env: map[string]string{
				"GITHUB_REPOSITORY": "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{
				"GITHUB_REPOSITORY",
				"GITHUB_WORKFLOW",
				"GITHUB_JOB",
				"GITHUB_RUN_ID",
				"GITHUB_RUN_ATTEMPT",
				"GITHUB_SHA",
				"GITHUB_REF",
				"GITHUB_REF_NAME",
				"GITHUB_BASE_REF",
				"GITHUB_HEAD_REF",
				"GITHUB_ACTOR",
				"GITHUB_ACTOR_ID",
				"RUNNER_ARCH",
				"RUNNER_OS",
				"RUNNER_NAME",
				"RUNNER_ENVIRONMENT",
			} {
				t.Setenv(key, "")
			}

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			meta, err := newFromGithubEnv()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, meta)

			if tt.assertion != nil {
				tt.assertion(t, meta)
			}
		})
	}
}
