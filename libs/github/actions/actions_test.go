package actions

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskSecretValues(t *testing.T) {
	t.Run("single-line secrets, empty values skipped", func(t *testing.T) {
		output := captureStdout(t, func() {
			MaskSecretValues([]string{"first-secret", "", "second-secret"})
		})

		expected := "\n" +
			"::add-mask::first-secret\n" +
			"::add-mask::Zmlyc3Qtc2VjcmV0\n" +
			"::add-mask::Zmlyc3Qtc2VjcmV0\n" +
			"::add-mask::second-secret\n" +
			"::add-mask::c2Vjb25kLXNlY3JldA==\n" +
			"::add-mask::c2Vjb25kLXNlY3JldA==\n"
		require.Equal(t, expected, output)
	})

	t.Run("multi-line secret is masked line by line", func(t *testing.T) {
		secret := "-----BEGIN KEY-----\nline-two\n-----END KEY-----"
		output := captureStdout(t, func() {
			MaskSecretValues([]string{secret})
		})

		stdEncoded := base64.StdEncoding.EncodeToString([]byte(secret))
		urlEncoded := base64.URLEncoding.EncodeToString([]byte(secret))
		expected := "\n" +
			"::add-mask::-----BEGIN KEY-----\n" +
			"::add-mask::line-two\n" +
			"::add-mask::-----END KEY-----\n" +
			"::add-mask::" + stdEncoded + "\n" +
			"::add-mask::" + urlEncoded + "\n"
		require.Equal(t, expected, output)
	})
}

// captureStdout redirects os.Stdout for the duration of fn and returns everything
// written to it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = writePipe
	t.Cleanup(func() {
		os.Stdout = originalStdout
	})

	fn()

	require.NoError(t, writePipe.Close())

	var output bytes.Buffer
	_, err = io.Copy(&output, readPipe)
	require.NoError(t, err)
	require.NoError(t, readPipe.Close())

	return output.String()
}

func TestWriteGithubEnv(t *testing.T) {
	t.Run("returns error when env file is not configured", func(t *testing.T) {
		t.Setenv(GithubEnv, "")

		err := WriteGithubEnv(map[string]string{"KEY": "value"})
		require.EqualError(t, err, "GITHUB_ENV environment variable not set")
	})

	t.Run("writes variables to configured file", func(t *testing.T) {
		envFile := filepath.Join(t.TempDir(), "github_env.txt")
		t.Setenv(GithubEnv, envFile)

		err := WriteGithubEnv(map[string]string{"KEY": "value"})
		require.NoError(t, err)

		contents, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assertMultilineAssignment(t, string(contents), "KEY", "value")
	})
}

func TestWriteGithubOutput(t *testing.T) {
	t.Run("returns error when output file is not configured", func(t *testing.T) {
		t.Setenv(GithubOutput, "")

		err := WriteGithubOutput(map[string]string{"KEY": "value"})
		require.EqualError(t, err, "GITHUB_OUTPUT environment variable not set")
	})

	t.Run("writes outputs to configured file", func(t *testing.T) {
		outputFile := filepath.Join(t.TempDir(), "github_output.txt")
		t.Setenv(GithubOutput, outputFile)

		err := WriteGithubOutput(map[string]string{"KEY": "value"})
		require.NoError(t, err)

		contents, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assertMultilineAssignment(t, string(contents), "KEY", "value")
	})
}

func TestWriteGithubState(t *testing.T) {
	t.Run("returns error when state file is not configured", func(t *testing.T) {
		t.Setenv(GithubState, "")

		err := WriteGithubState(map[string]string{"KEY": "value"})
		require.EqualError(t, err, "GITHUB_STATE environment variable not set")
	})

	t.Run("writes state to configured file", func(t *testing.T) {
		stateFile := filepath.Join(t.TempDir(), "github_state.txt")
		t.Setenv(GithubState, stateFile)

		err := WriteGithubState(map[string]string{"KEY": "value"})
		require.NoError(t, err)

		contents, err := os.ReadFile(stateFile)
		require.NoError(t, err)
		assertMultilineAssignment(t, string(contents), "KEY", "value")
	})
}

func assertMultilineAssignment(t *testing.T, contents, key, value string) {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(contents, "\n"), "\n")
	require.Len(t, lines, 3)
	require.True(t, strings.HasPrefix(lines[0], key+"<<ghadelimiter_"))
	require.Equal(t, value, lines[1])
	require.Equal(t, strings.TrimPrefix(lines[0], key+"<<"), lines[2])
}
