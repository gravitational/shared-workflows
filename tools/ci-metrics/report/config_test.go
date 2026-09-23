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

package report

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig puts body in a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "reports.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestLoadConfigFallsBackToDefault(t *testing.T) {
	t.Parallel()

	// No flag and no env var should still give a runnable config, so that
	// trying the tool needs no file.
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, []string{FlakyDailyName, FlakyRollupName}, cfg.ReportNames())
	for name, rc := range cfg.Reports {
		assert.Equal(t, []string{"stdout"}, rc.Reporters, "report %q", name)
	}
	assert.Equal(t, defaultWindowDays, cfg.Window.Days)
	assert.Equal(t, "meta_v2_parquet", cfg.Tables.Meta)
	assert.Equal(t, "testcases_v2_parquet", cfg.Tables.Testcases)
}

func TestLoadConfigFromFile(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
tables:
  meta: meta_v3
  testcases: testcases_v3
window:
  days: 7
reporters:
  console:
    type: stdout
    max_rows: 5
reports:
  flaky_rollup:
    reporters: [console]
    params:
      min_execs: 25
      top: 3
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "meta_v3", cfg.Tables.Meta)
	assert.Equal(t, "testcases_v3", cfg.Tables.Testcases)
	assert.Equal(t, 7, cfg.Window.Days)
	require.Contains(t, cfg.Reporters, "console")
	assert.Equal(t, 5, cfg.Reporters["console"].MaxRows)

	assert.Equal(t, []string{FlakyRollupName}, cfg.ReportNames())
	assert.Equal(t, []string{"console"}, cfg.Reports[FlakyRollupName].Reporters)
}

func TestDecodeParams(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
reporters:
  stdout:
    type: stdout
reports:
  flaky_rollup:
    params:
      min_execs: 25
      smoothing: 7.5
      top: 3
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	def, ok := Get(FlakyRollupName)
	require.True(t, ok)

	params, err := cfg.Reports[FlakyRollupName].DecodeParams(FlakyRollupName, def)
	require.NoError(t, err)

	flaky, ok := params.(*FlakyParams)
	require.True(t, ok, "got %T", params)
	assert.Equal(t, 25, flaky.MinExecs)
	assert.InDelta(t, 7.5, flaky.Smoothing, 1e-9)
	assert.Equal(t, 3, flaky.Top)
}

func TestDecodeParamsAbsentGivesDefaults(t *testing.T) {
	t.Parallel()

	def, ok := Get(FlakyRollupName)
	require.True(t, ok)

	params, err := ReportConfig{}.DecodeParams(FlakyRollupName, def)
	require.NoError(t, err)

	flaky, ok := params.(*FlakyParams)
	require.True(t, ok)
	assert.Equal(t, 0, flaky.MinExecs)
}

func TestDecodeParamsRejectsUnknownField(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
reporters:
  stdout:
    type: stdout
reports:
  flaky_rollup:
    params:
      min_exec: 25
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	def, ok := Get(FlakyRollupName)
	require.True(t, ok)
	_, err = cfg.Reports[FlakyRollupName].DecodeParams(FlakyRollupName, def)
	assert.Error(t, err)
}

func TestLoadConfigRejectsUnknownTopLevelField(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
reportres:
  stdout:
    type: stdout
reports:
  flaky_rollup: {}
`)

	_, err := LoadConfig(path)
	assert.Error(t, err)
}

func TestLoadConfigValidation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body      string
		assertErr require.ErrorAssertionFunc
	}{
		"no reports": {
			body:      "reporters:\n  stdout:\n    type: stdout\n",
			assertErr: require.Error,
		},
		"unknown report": {
			body:      "reports:\n  nonsense: {}\n",
			assertErr: require.Error,
		},
		"reporter without type": {
			body:      "reporters:\n  a: {}\nreports:\n  flaky_rollup: {}\n",
			assertErr: require.Error,
		},
		"undefined reporter reference": {
			body: "reporters:\n  a:\n    type: stdout\n" +
				"reports:\n  flaky_rollup:\n    reporters: [b]\n",
			assertErr: require.Error,
		},
		"negative window": {
			body:      "window:\n  days: -1\nreports:\n  flaky_rollup: {}\n",
			assertErr: require.Error,
		},
		"negative max_rows": {
			body: "reporters:\n  a:\n    type: stdout\n    max_rows: -1\n" +
				"reports:\n  flaky_rollup: {}\n",
			assertErr: require.Error,
		},
		"invalid table identifier": {
			body:      "tables:\n  meta: \"m; DROP TABLE x\"\nreports:\n  flaky_rollup: {}\n",
			assertErr: require.Error,
		},
		"minimal valid": {
			body:      "reports:\n  flaky_rollup: {}\n",
			assertErr: require.NoError,
		},
		"both reports": {
			body:      "reports:\n  flaky_rollup: {}\n  flaky_daily: {}\n",
			assertErr: require.NoError,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadConfig(writeConfig(t, tt.body))
			tt.assertErr(t, err)
		})
	}
}

func TestLoadConfigDefaultsReportersToAll(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, `
reporters:
  b:
    type: stdout
  a:
    type: stdout
reports:
  flaky_rollup: {}
`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, cfg.Reports[FlakyRollupName].Reporters)
}

func TestLoadConfigFromEnvBody(t *testing.T) {
	t.Setenv(EnvConfigBody, "window:\n  days: 3\nreports:\n  flaky_rollup: {}\n")
	cfg, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.Window.Days)
}

func TestLoadConfigMissingFileIsAnError(t *testing.T) {
	t.Parallel()
	_, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.Error(t, err)
}

func TestDefaultConfigIsValid(t *testing.T) {
	t.Parallel()
	require.NoError(t, DefaultConfig().checkAndSetDefaults())
}

func TestExampleConfigParses(t *testing.T) {
	t.Parallel()

	cfg, err := LoadConfig(filepath.Join("..", "docs", "reports.example.yaml"))
	require.NoError(t, err)

	// The example is expected to cover every registered report, so a new one
	// that is never documented fails here.
	assert.Equal(t, Names(), cfg.ReportNames())

	for name, rc := range cfg.Reports {
		def, ok := Get(name)
		require.True(t, ok, "report %q", name)

		params, err := rc.DecodeParams(name, def)
		require.NoError(t, err)

		flaky, ok := params.(*FlakyParams)
		require.True(t, ok, "report %q got %T", name, params)
		require.NoError(t, flaky.checkAndSetDefaults())
	}
}
