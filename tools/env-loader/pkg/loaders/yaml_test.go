/*
 *  Copyright 2024 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package loaders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
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
		resultingValues map[string]values.Value
		checkError      require.ErrorAssertionFunc
	}{
		{
			testFileName:    "empty.yaml",
			resultingValues: map[string]values.Value{},
		},
		{
			testFileName:    "empty_docstring.yaml",
			resultingValues: map[string]values.Value{},
		},
		{
			testFileName: "single_value.yaml",
			resultingValues: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
			},
		},
		{
			testFileName: "no_value.yaml",
			resultingValues: map[string]values.Value{
				"key": {},
			},
		},
		{
			testFileName: "multiline.yaml",
			resultingValues: map[string]values.Value{
				"key": {UnderlyingValue: "some\nmultiline\nvalue\n"},
			},
		},
		{
			testFileName: "multiple_values.yaml",
			resultingValues: map[string]values.Value{
				"key1": {UnderlyingValue: "value1"},
				"key2": {UnderlyingValue: "value2"},
			},
		},
		{
			testFileName: "anchor.yaml",
			resultingValues: map[string]values.Value{
				"key1": {UnderlyingValue: "value"},
				"key2": {UnderlyingValue: "value"},
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
			"maps for test file %q are not equal", testCase.testFileName)
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
		resultingValues map[string]values.Value
		checkError      require.ErrorAssertionFunc
	}{
		{
			testFileName:   "single_value.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
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
			resultingValues: map[string]values.Value{},
		},
		{
			testFileName:   "multiple_keys.sops.yaml",
			ageKeyFileName: "key2.age",
			resultingValues: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
			},
		},
		{
			desc:           "key 1",
			testFileName:   "multiple_keys.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
			},
		},
		{
			desc:           "key 2",
			testFileName:   "mixed_unencrypted.sops.yaml",
			ageKeyFileName: "key1.age",
			resultingValues: map[string]values.Value{
				"key1":             {UnderlyingValue: "value1"},
				"key2_unencrypted": {UnderlyingValue: "value2_unencrypted"},
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

		// All output value should be marked as secret. Set this here for
		// convenience and test case readability
		for key, value := range testCase.resultingValues {
			value.ShouldMask = true
			testCase.resultingValues[key] = value
		}

		// Run
		builtValues, err := loader.GetEnvironmentValues(rawContent)
		testCase.checkError(t, err, "file: %q, description: %s",
			testCase.testFileName, testCase.desc)
		require.Equal(t, testCase.resultingValues, builtValues,
			"maps for testfile %q are not equal (description: %s)",
			testCase.testFileName, testCase.desc)

		for _, value := range builtValues {
			require.True(t, value.ShouldMask,
				"value for test file %q was not marked as secret", testCase.testFileName)
		}
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
