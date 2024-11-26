package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/alecthomas/kingpin/v2"
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
		expectedValues map[string]string
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
			expectedValues: map[string]string{
				"setLevel":        "set level",
				"envLevelCommon1": "env level",
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
			expectedValues: map[string]string{
				"setLevel":        "set level",
				"setLevelCommon":  "testing1 level",
				"envLevelCommon1": "env level",
				"envLevelCommon2": "set level",
				"topLevelCommon1": "top level",
				"topLevelCommon2": "env level",
			},
		},
		{
			desc: "specific env",
			c: &config{
				EnvironmentsDirectory: filepath.Join("..", "pkg", "testdata", "repos", "basic repo", ".environments"),
				Environment:           "env1",
			},
			expectedValues: map[string]string{
				"envLevelCommon1": "env level",
				"envLevelCommon2": "env level",
				"topLevelCommon1": "top level",
				"topLevelCommon2": "env level",
			},
		},
	}

	for _, test := range tests {
		actualValues, err := getRequestedEnvValues(test.c)
		require.NoError(t, err)
		require.EqualValues(t, actualValues, test.expectedValues)
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		desc           string
		c              *config
		expectedOutput string
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
			expectedOutput: "envLevelCommon1=env level\nsetLevel=set level\n",
		},
	}

	for _, test := range tests {
		// Setup to capture stdout
		var output bytes.Buffer
		var writtenLength int
		var capturedErr error
		outputPrinter = func(a ...any) (n int, err error) {
			writtenLength, capturedErr = fmt.Fprint(&output, a...)
			return writtenLength, capturedErr
		}

		err := run(test.c)

		require.NoError(t, err)
		require.NoError(t, capturedErr)
		require.Equal(t, writtenLength, len(test.expectedOutput))
		require.Equal(t, output.String(), test.expectedOutput)
	}
}
