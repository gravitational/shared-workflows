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
	"errors"
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

type dummyLoader struct {
	canDecode bool
	envVals   map[string]values.Value
	err       error
}

func (*dummyLoader) Name() string {
	return "dummy loader"
}

func (dl *dummyLoader) CanGetEnvironmentValues(bytes []byte) bool {
	// return dl.canGetEnvironmentValues(bytes)
	return dl.canDecode
}

func (dl *dummyLoader) GetEnvironmentValues(bytes []byte) (map[string]values.Value, error) {
	// return dl.getEnvironmentValues(bytes)
	return dl.envVals, dl.err
}

func createCanDecodeLoader(canDecode bool) Loader {
	return &dummyLoader{canDecode: canDecode}
}

func TestSubloader_CanGetEnvironmentValues(t *testing.T) {
	testCases := []struct {
		desc          string
		childLoaders  []Loader
		expectedValue bool
	}{
		{
			desc: "no loaders",
		},
		{
			desc: "single loader can decode",
			childLoaders: []Loader{
				createCanDecodeLoader(true),
			},
			expectedValue: true,
		},
		{
			desc: "single loader cannot decode",
			childLoaders: []Loader{
				createCanDecodeLoader(false),
			},
		},
		{
			desc: "multiple loaders, second can decode",
			childLoaders: []Loader{
				createCanDecodeLoader(false),
				createCanDecodeLoader(true),
				createCanDecodeLoader(false),
			},
			expectedValue: true,
		},
		{
			desc: "multiple loaders, none can decode",
			childLoaders: []Loader{
				createCanDecodeLoader(false),
				createCanDecodeLoader(false),
				createCanDecodeLoader(false),
			},
		},
	}

	for _, testCase := range testCases {
		loader := NewSubLoader(testCase.childLoaders...)
		actualValue := loader.CanGetEnvironmentValues(nil)
		require.Equal(t, testCase.expectedValue, actualValue, testCase.desc)
	}
}

func TestSubloader_GetEnvironmentValues(t *testing.T) {
	testEnvVals := map[string]values.Value{"key": {UnderlyingValue: "value"}}
	testErr := errors.New("dummy error")

	testCases := []struct {
		desc         string
		childLoaders []Loader
		checkError   require.ErrorAssertionFunc
	}{
		{
			desc:       "no loader",
			checkError: require.Error,
		},
		{
			desc: "single loader, can decode",
			childLoaders: []Loader{
				&dummyLoader{canDecode: true, envVals: testEnvVals},
			},
		},
		{
			desc: "single loader, cannot decode",
			childLoaders: []Loader{
				&dummyLoader{canDecode: false},
			},
			checkError: require.Error,
		},
		{
			desc: "single loader, fail to decode",
			childLoaders: []Loader{
				&dummyLoader{canDecode: true, err: testErr},
			},
			checkError: require.Error,
		},
		{
			desc: "multiple loaders, second can decode",
			childLoaders: []Loader{
				&dummyLoader{canDecode: false, err: testErr},
				&dummyLoader{canDecode: true, envVals: testEnvVals},
				&dummyLoader{canDecode: false, err: testErr},
			},
		},
	}

	for _, testCase := range testCases {
		loader := NewSubLoader(testCase.childLoaders...)
		actualValues, err := loader.GetEnvironmentValues(nil)

		var expectedValues map[string]values.Value
		if testCase.checkError == nil {
			testCase.checkError = require.NoError
			expectedValues = testEnvVals
		}

		testCase.checkError(t, err, testCase.desc)
		require.Equal(t, expectedValues, actualValues, testCase.desc)
	}
}
