package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

func TestParseCli(t *testing.T) {
	parseCLI([]string{})

	flags := kingpin.CommandLine.Model().Flags
	flagEnvVars := make([]string, 0, len(flags))
	for _, flag := range flags {
		if flag.Envar != "" {
			flagEnvVars = append(flagEnvVars, flag.Envar)
		}
	}

	uniqueFlagEnvVars := slices.Compact(slices.Clone(flagEnvVars))

	require.ElementsMatch(t, flagEnvVars, uniqueFlagEnvVars, "not all flag env vars are unique")
}

func TestGetRequestedEnvValues(t *testing.T) {
	tests := []struct {
		desc           string
		c              *config
		expectedValues map[string]values.Value
	}{
		{
			desc: "specific values",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
				ValueSets: []string{
					"testing1",
				},
				Values: []string{
					"setLevel",
					"envLevelCommon1",
				},
			},
			expectedValues: map[string]values.Value{
				"setLevel":        {UnderlyingValue: "set level"},
				"envLevelCommon1": {UnderlyingValue: "env level"},
			},
		},
		{
			desc: "full value set",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
				ValueSets: []string{
					"testing1",
				},
			},
			expectedValues: map[string]values.Value{
				"setLevel":        {UnderlyingValue: "set level"},
				"setLevelCommon":  {UnderlyingValue: "testing1 level"},
				"envLevelCommon1": {UnderlyingValue: "env level"},
				"envLevelCommon2": {UnderlyingValue: "set level"},
				"topLevelCommon1": {UnderlyingValue: "top level"},
				"topLevelCommon2": {UnderlyingValue: "env level"},
			},
		},
		{
			desc: "specific env",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
			},
			expectedValues: map[string]values.Value{
				"envLevelCommon1": {UnderlyingValue: "env level"},
				"envLevelCommon2": {UnderlyingValue: "env level"},
				"topLevelCommon1": {UnderlyingValue: "top level"},
				"topLevelCommon2": {UnderlyingValue: "env level"},
			},
		},
	}

	for _, test := range tests {
		actualValues, err := getRequestedEnvValues(test.c)
		require.NoError(t, err)
		require.EqualValues(t, test.expectedValues, actualValues)
	}
}

func TestRun(t *testing.T) {
	os.Setenv("SOPS_AGE_KEY_FILE", filepath.Join("..", "pkg", "loaders", "testdata", "key1.age"))

	tests := []struct {
		desc            string
		c               *config
		expectedOutputs []string // Must support multiple options due to map iteration randomness
	}{
		{
			desc: "specific values",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
				ValueSets: []string{
					"testing1",
				},
				Values: []string{
					"setLevel",
					"envLevelCommon1",
				},
				Writer: "dotenv",
			},
			expectedOutputs: []string{
				"envLevelCommon1=env level\nsetLevel=set level\n",
				"setLevel=set level\nenvLevelCommon1=env level\n",
			},
		},
		{
			desc: "secret masked values",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
				Writer:                "gha-mask",
				ValueSets: []string{
					"secrets",
				},
			},
			expectedOutputs: []string{
				"::add-mask::value1\n::add-mask::value2_unencrypted\n",
				"::add-mask::value2_unencrypted\n::add-mask::value1\n",
			},
		},
	}

	for _, test := range tests {
		// Setup to capture stdout
		var outputBytes bytes.Buffer
		outputWriter = &outputBytes

		err := run(test.c)

		output := outputBytes.String()

		require.NoError(t, err)
		require.Contains(t, test.expectedOutputs, output)
	}
}
