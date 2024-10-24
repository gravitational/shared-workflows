package writers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Tests that apply to all writer implementations

func TestWriterName(t *testing.T) {
	for listedName, writer := range FromName {
		require.NotEmpty(t, listedName, "writer name for %v is empty", writer)
		require.Equal(t, listedName, writer.Name(), "writer name for %v is not consistent", writer)
	}
}

func TestWriterFormatValid(t *testing.T) {
	testCases := []struct {
		desc       string
		values     map[string]string
		canBeEmpty bool
		checkError require.ErrorAssertionFunc
	}{
		{
			desc: "single value",
			values: map[string]string{
				"key": "value",
			},
		},
		{
			desc: "multiple values",
			values: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			desc: "key with empty value",
			values: map[string]string{
				"key": "",
			},
			canBeEmpty: true,
		},
		{
			desc:       "no values",
			canBeEmpty: true,
		},
		{
			desc: "empty key",
			values: map[string]string{
				"": "value",
			},
			checkError: require.Error,
			canBeEmpty: true,
		},
	}

	for name, writer := range FromName {
		for _, testCase := range testCases {
			formattedStr, err := writer.FormatEnvironmentValues(testCase.values)

			if testCase.checkError == nil {
				testCase.checkError = require.NoError
			}

			testCase.checkError(t, err, "writer %q failed with test case %q", name, testCase.desc)
			if !testCase.canBeEmpty {
				require.NotEmpty(t, formattedStr, "writer output for %q is empty", name)
			}
		}
	}
}
