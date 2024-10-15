package loaders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const AGE_KEY_ENV_VAR_NAME = "SOPS_AGE_KEY_FILE"

func TestYamlSubloader_Names(t *testing.T) {
	loaders := NewYAMLLoader().SubLoaders
	for _, loader := range loaders {
		require.NotEmpty(t, loader.Name(), "%#v", loader)
	}

}

func readTestFile(t *testing.T, fileName string) []byte {
	testFilePath := filepath.Join("testdata", fileName)
	rawContent, err := os.ReadFile(testFilePath)
	require.NoError(t, err, "failed to load test file %q", testFilePath)

	return rawContent
}

func TestPlainYamlSubloader_GetEnvironmentValues(t *testing.T) {
	testCases := []struct {
		testFileName    string
		resultingValues map[string]string
		checkError      require.ErrorAssertionFunc
	}{
		{
			testFileName:    "empty.yaml",
			resultingValues: map[string]string{},
		},
		{
			testFileName:    "empty_docstring.yaml",
			resultingValues: map[string]string{},
		},
		{
			testFileName: "single_value.yaml",
			resultingValues: map[string]string{
				"key": "value",
			},
		},
		{
			testFileName: "no_value.yaml",
			resultingValues: map[string]string{
				"key": "",
			},
		},
		{
			testFileName: "multiline.yaml",
			resultingValues: map[string]string{
				"key": "some\nmultiline\nvalue\n",
			},
		},
		{
			testFileName: "multiple_values.yaml",
			resultingValues: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			testFileName: "anchor.yaml",
			resultingValues: map[string]string{
				"key1": "value",
				"key2": "value",
			},
		},
		{
			testFileName: "subkey.yaml",
			checkError:   require.Error,
		},
	}

	for _, testCase := range testCases {
		// Setup
		loader := plainYAMLSubloader{}

		rawContent := readTestFile(t, testCase.testFileName)

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		// Run tests
		builtValues, err := loader.GetEnvironmentValues(rawContent)
		testCase.checkError(t, err, "file: %q", testCase.testFileName)
		require.Equal(t, testCase.resultingValues, builtValues,
			"maps for testfile %q are not equal", testCase.testFileName)
	}
}

func TestPlainYamlSubloader_CanEnvironmentValues(t *testing.T) {
	testCases := []struct {
		desc         string
		testBytes    []byte
		testFileName string
		canDecode    bool
	}{
		{
			desc:         "YAML",
			testFileName: "single_value.yaml",
			canDecode:    true,
		},
		{
			desc:         "JSON",
			testFileName: "single_value.json",
			canDecode:    true,
		},
		{
			desc:      "no bytes",
			testBytes: []byte{},
		},
		{
			desc:      "nil bytes",
			testBytes: nil,
		},
		{
			desc:      "unsupported bytes",
			testBytes: []byte("abc123"),
		},
	}

	for _, testCase := range testCases {
		// Setup
		loader := plainYAMLSubloader{}

		if testCase.testBytes == nil && testCase.testFileName != "" {
			testCase.testBytes = readTestFile(t, testCase.testFileName)
		}

		// Run tests
		canGetValues := loader.CanGetEnvironmentValues(testCase.testBytes)
		require.Equal(t, testCase.canDecode, canGetValues, testCase.desc)
	}
}

func TestSOPSYamlSubloader_GetEnvironmentValues(t *testing.T) {
	testCases := []struct {
		desc            string
		testFileName    string
		ageKeyFileName  string
		resultingValues map[string]string
		checkError      require.ErrorAssertionFunc
	}{
		{
			testFileName:   "single_value.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]string{
				"key": "value",
			},
		},
		{
			desc:           "wrong key",
			testFileName:   "single_value.sops.yaml",
			ageKeyFileName: "key2.age",
			checkError:     require.Error,
		},
		{
			desc:         "no key",
			testFileName: "single_value.sops.yaml",
			checkError:   require.Error,
		},
		{
			testFileName:    "empty.sops.yaml",
			ageKeyFileName:  "key1.age",
			resultingValues: map[string]string{},
		},
		{
			testFileName:   "multiple_keys.sops.yaml",
			ageKeyFileName: "key2.age",
			resultingValues: map[string]string{
				"key": "value",
			},
		},
		{
			desc:           "key 1",
			testFileName:   "multiple_keys.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]string{
				"key": "value",
			},
		},
		{
			desc:           "key 2",
			testFileName:   "mixed_unencrypted.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]string{
				"key1":             "value1",
				"key2_unencrypted": "value2_unencrypted",
			},
		},
		{
			testFileName:   "malformed.sops.yaml",
			ageKeyFileName: "key1.age",
			checkError:     require.Error,
		},
		{
			testFileName:   "wrong_mac.sops.yaml",
			ageKeyFileName: "key1.age",
			checkError:     require.Error,
		},
	}

	for _, testCase := range testCases {
		// Setup
		loader := SOPSYAMLSubloader{}

		rawContent := readTestFile(t, testCase.testFileName)

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		if testCase.ageKeyFileName != "" {
			os.Setenv(AGE_KEY_ENV_VAR_NAME,
				filepath.Join("testdata", testCase.ageKeyFileName))
		} else {
			os.Unsetenv(AGE_KEY_ENV_VAR_NAME)
		}

		// Run tests
		builtValues, err := loader.GetEnvironmentValues(rawContent)
		testCase.checkError(t, err, "file: %q, description: %s",
			testCase.testFileName, testCase.desc)
		require.Equal(t, testCase.resultingValues, builtValues,
			"maps for testfile %q are not equal (description: %s)",
			testCase.testFileName, testCase.desc)
	}
}

func TestSOPSYamlSubloader_CanEnvironmentValues(t *testing.T) {
	testCases := []struct {
		desc         string
		testBytes    []byte
		testFileName string
		canDecode    bool
	}{
		{
			desc:         "YAML (with SOPS header)",
			testFileName: "single_value.sops.yaml",
			canDecode:    true,
		},
		{
			desc:         "YAML (no SOPS header)",
			testFileName: "single_value.yaml",
		},
		{
			desc:         "JSON (with SOPS header)",
			testFileName: "single_value.sops.json",
			canDecode:    true,
		},
		{
			desc:         "JSON (no SOPS header)",
			testFileName: "single_value.json",
		},
		{
			desc:      "no bytes",
			testBytes: []byte{},
		},
		{
			desc:      "nil bytes",
			testBytes: nil,
		},
		{
			desc:      "unsupported bytes",
			testBytes: []byte("abc123"),
		},
	}

	for _, testCase := range testCases {
		// Setup
		loader := SOPSYAMLSubloader{}

		if testCase.testBytes == nil && testCase.testFileName != "" {
			testFilePath := filepath.Join("testdata", testCase.testFileName)
			rawContent, err := os.ReadFile(testFilePath)
			require.NoError(t, err, "failed to load test file %q", testFilePath)
			testCase.testBytes = rawContent
		}

		// Run tests
		canGetValues := loader.CanGetEnvironmentValues(testCase.testBytes)
		require.Equal(t, testCase.canDecode, canGetValues, testCase.desc)
	}
}

func TestYaml_Name(t *testing.T) {
	loader := NewYAMLLoader()
	require.NotEmpty(t, loader.Name())
}
