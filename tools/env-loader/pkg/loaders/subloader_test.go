package loaders

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type dummyLoader struct {
	canDecode bool
	envVals   map[string]string
	err       error
	// canGetEnvironmentValues func([]byte) bool
	// getEnvironmentValues    func([]byte) (map[string]string, error)
}

func (*dummyLoader) Name() string {
	return "dummy loader"
}

func (dl *dummyLoader) CanGetEnvironmentValues(bytes []byte) bool {
	// return dl.canGetEnvironmentValues(bytes)
	return dl.canDecode
}

func (dl *dummyLoader) GetEnvironmentValues(bytes []byte) (map[string]string, error) {
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
	testEnvVals := map[string]string{"key": "value"}
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

		var expectedValues map[string]string
		if testCase.checkError == nil {
			testCase.checkError = require.NoError
			expectedValues = testEnvVals
		}

		testCase.checkError(t, err, testCase.desc)
		require.Equal(t, expectedValues, actualValues, testCase.desc)
	}
}
