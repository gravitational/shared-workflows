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

// Tests that apply to all writer implementations

func TestWriterName(t *testing.T) {
	for listedName, writer := range FromName {
		require.NotEmpty(t, listedName, "writer name for %v is empty", writer)
		require.Equal(t, listedName, writer.Name(), "writer name for %v is not consistent", writer)
	}
}

// Cover common test cases that all writers should pass
// Most test cases should be writer-specific
func TestWriterFormatValid(t *testing.T) {
	testCases := []struct {
		desc   string
		values map[string]values.Value
	}{
		{
			desc: "empty key",
			values: map[string]values.Value{
				"": {UnderlyingValue: "value"},
			},
		},
		{
			desc: "empty key, secret value",
			values: map[string]values.Value{
				"": {UnderlyingValue: "secret value", ShouldMask: true},
			},
		},
		{
			desc: "empty secret key, mixed",
			values: map[string]values.Value{
				"key1": {UnderlyingValue: "secret value1", ShouldMask: true},
				"":     {UnderlyingValue: "secret value2", ShouldMask: true},
				"key3": {UnderlyingValue: "secret value3", ShouldMask: true},
			},
		},
	}

	for name, writer := range FromName {
		for _, testCase := range testCases {
			_, err := writer.FormatEnvironmentValues(testCase.values)

			require.Error(t, err, "writer %q failed with test case %q", name, testCase.desc)

			for _, value := range testCase.values {
				if value.ShouldMask {
					require.NotContains(t, err.Error(), value.UnderlyingValue)
				}
			}
		}
	}
}
