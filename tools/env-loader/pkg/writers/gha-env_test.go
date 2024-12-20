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

package writers

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

func TestGenerateMultilineDelimiter(t *testing.T) {
	testCases := []string{
		"value",
		"multiline\nvalue",
		"multiline\nvalue\nwith\nnewline\n",
		"\n",
		"\n\n\n",
		fmt.Sprintf("\n%s", delimiterPrefix),
		fmt.Sprintf("\n%s\n", delimiterPrefix),
		"",
		delimiterPrefix,
		fmt.Sprintf("%s_%s", delimiterPrefix, uuid.NewString()),
	}

	for _, testCase := range testCases {
		actualDelimiter := generateMultilineDelimiter(testCase)
		require.NotEqual(t, testCase, actualDelimiter)
	}
}

func TestGHAEnvFormat(t *testing.T) {
	testCases := []struct {
		desc            string
		values          map[string]values.Value
		expectedOutputs []string
		checkError      require.ErrorAssertionFunc
	}{
		{
			desc: "single value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
			},
			expectedOutputs: []string{
				"key=value\n",
			},
		},
		{
			desc: "multiple values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "value1"},
				"key2": {UnderlyingValue: "value2"},
			},
			expectedOutputs: []string{
				"key1=value1\nkey2=value2\n",
				"key2=value2\nkey1=value1\n",
			},
		},
		{
			desc: "key with empty value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: ""},
			},
			expectedOutputs: []string{
				"key=\n",
			},
		},
		{
			desc: "no values",
		},
		{
			desc: "empty key",
			values: map[string]values.Value{
				"": {UnderlyingValue: "value"},
			},
			checkError: require.Error,
		},
		{
			desc: "multiline value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "multiline\nvalue"},
			},
			expectedOutputs: []string{
				"key<<EOF\nmultiline\nvalue\nEOF\n",
			},
		},
		{
			desc: "multiline value with new line at end",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "multiline\nvalue\n"},
			},
			expectedOutputs: []string{
				"key<<EOF\nmultiline\nvalue\n\nEOF\n",
			},
		},
		{
			desc: "multiple multiline values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "multiline\nvalue1\n"},
				"key2": {UnderlyingValue: "multiline\nvalue2"},
			},
			expectedOutputs: []string{
				"key1<<EOF\nmultiline\nvalue1\n\nEOF\nkey2<<EOF\nmultiline\nvalue2\nEOF\n",
				"key2<<EOF\nmultiline\nvalue2\nEOF\nkey1<<EOF\nmultiline\nvalue1\n\nEOF\n",
			},
		},
		{
			desc: "multiple mixed multiline values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "multiline\nvalue1\n"},
				"key2": {UnderlyingValue: "value2"},
				"key3": {UnderlyingValue: "multiline\nvalue3"},
			},
			expectedOutputs: []string{
				"key1<<EOF\nmultiline\nvalue1\n\nEOF\nkey2=value2\nkey3<<EOF\nmultiline\nvalue3\nEOF\n",
				"key1<<EOF\nmultiline\nvalue1\n\nEOF\nkey3<<EOF\nmultiline\nvalue3\nEOF\nkey2=value2\n",
				"key2=value2\nkey1<<EOF\nmultiline\nvalue1\n\nEOF\nkey3<<EOF\nmultiline\nvalue3\nEOF\n",
				"key2=value2\nkey3<<EOF\nmultiline\nvalue3\nEOF\nkey1<<EOF\nmultiline\nvalue1\n\nEOF\n",
				"key3<<EOF\nmultiline\nvalue3\nEOF\nkey1<<EOF\nmultiline\nvalue1\n\nEOF\nkey2=value2\n",
				"key3<<EOF\nmultiline\nvalue3\nEOF\nkey2=value2\nkey1<<EOF\nmultiline\nvalue1\n\nEOF\n",
			},
		},
	}

	writer := NewGHAEnvWriter()
	for _, testCase := range testCases {
		formattedStr, err := writer.FormatEnvironmentValues(testCase.values)

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		testCase.checkError(t, err, "writer failed with test case %q", testCase.desc)

		// Using separate functions makes debugging easier
		switch len(testCase.expectedOutputs) {
		case 0:
			require.Empty(t, formattedStr, "output value was not empty for test case %q", testCase.desc)
		case 1:
			require.Equal(t, testCase.expectedOutputs[0], formattedStr,
				"output value did not match expected value for test case %q", testCase.desc)
		default:
			require.Contains(t, testCase.expectedOutputs, formattedStr,
				"output value did not match any expected for test case %q", testCase.desc)
		}
	}
}
