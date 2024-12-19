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
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

func TestGHAMaskFormat(t *testing.T) {
	testCases := []struct {
		desc            string
		values          map[string]values.Value
		expectedOutputs []string // Multiple values supported due to map iteration randomness
		shouldError     bool
	}{
		{
			desc: "single value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "value"},
			},
		},
		{
			desc: "multiple values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "value1"},
				"key2": {UnderlyingValue: "value2"},
			},
		},
		{
			desc: "key with empty value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: ""},
			},
		},
		{
			desc: "no values",
		},
		{
			desc: "single secret value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "secret value", ShouldMask: true},
			},
			expectedOutputs: []string{"::add-mask::secret value\n"},
		},
		{
			desc: "multiple secret values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "secret value1", ShouldMask: true},
				"key2": {UnderlyingValue: "secret value2", ShouldMask: true},
			},
			expectedOutputs: []string{
				"::add-mask::secret value1\n::add-mask::secret value2\n",
				"::add-mask::secret value2\n::add-mask::secret value1\n",
			},
		},
		{
			desc: "mixed secret values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "value1"},
				"key2": {UnderlyingValue: "secret value2", ShouldMask: true},
				"key3": {UnderlyingValue: "secret value3", ShouldMask: true},
				"key4": {UnderlyingValue: "value4"},
			},
			expectedOutputs: []string{
				"::add-mask::secret value2\n::add-mask::secret value3\n",
				"::add-mask::secret value3\n::add-mask::secret value2\n",
			},
		},
		{
			desc: "key with secret empty value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "", ShouldMask: true},
			},
		},
		{
			desc: "multiline secret value",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "secret \nvalue", ShouldMask: true},
			},
			expectedOutputs: []string{
				"::add-mask::secret \n::add-mask::value\n",
			},
		},
		{
			desc: "multiline secret value with new line at end",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "secret \nvalue\n", ShouldMask: true},
			},
			expectedOutputs: []string{
				"::add-mask::secret \n::add-mask::value\n",
			},
		},
		{
			desc: "multiline secret value with new lines in the middle",
			values: map[string]values.Value{
				"key": {UnderlyingValue: "secret \n\n\nvalue\n", ShouldMask: true},
			},
			expectedOutputs: []string{
				"::add-mask::secret \n::add-mask::value\n",
			},
		},
		{
			desc: "multiline secret value with additional values",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "secret \n\n\nvalue1\n", ShouldMask: true},
				"key2": {UnderlyingValue: "secret value2", ShouldMask: true},
				"key3": {UnderlyingValue: "secret \n\n\nvalue3\n", ShouldMask: true},
			},
			expectedOutputs: []string{
				"::add-mask::secret \n::add-mask::value1\n::add-mask::secret value2\n::add-mask::secret \n::add-mask::value3\n",
				"::add-mask::secret \n::add-mask::value1\n::add-mask::secret \n::secret value2\n::add-mask::value3\n::add-mask",
				"::add-mask::secret value2\n::add-mask::secret \n::add-mask::value1\n::add-mask::secret \n::add-mask::value3\n",
				"::add-mask::secret value2\n::add-mask::secret \n::add-mask::value3\n::add-mask::secret \n::add-mask::value1\n",
				"::add-mask::secret \n::add-mask::value3\n::add-mask::secret value2\n::add-mask::secret \n::add-mask::value1\n",
				"::add-mask::secret \n::add-mask::value3\n::add-mask::secret \n::add-mask::value1\n::add-mask::secret value2\n",
			},
		},
	}

	writer := NewGHAMaskWriter()
	for _, testCase := range testCases {
		formattedStr, err := writer.FormatEnvironmentValues(testCase.values)

		errFunc := require.NoError
		if testCase.shouldError {
			errFunc = require.Error
		}
		errFunc(t, err, "writer failed with test case %q", testCase.desc)

		// Ensure that no sensitive values are logged
		if testCase.shouldError {
			errMsg := err.Error()
			for _, value := range testCase.values {
				if value.ShouldMask {
					require.NotContains(t, errMsg, value.UnderlyingValue,
						"secret value %q logged for test case %q", value.UnderlyingValue,
						testCase.desc)
				}
			}
		}

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
