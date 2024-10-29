package writers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDotenvValidation(t *testing.T) {
	writer := NewDotenvWriter()
	testCases := []struct {
		desc       string
		key        string
		value      string
		checkError require.ErrorAssertionFunc
	}{
		{
			desc:       "valid value",
			key:        "key",
			value:      "value",
			checkError: require.NoError,
		},
		{
			desc:       "no value",
			key:        "key",
			checkError: require.NoError,
		},
		{
			desc:       "contains '_'",
			key:        "key_name",
			checkError: require.NoError,
		},
		{
			desc:       "all caps",
			key:        "KEY",
			checkError: require.NoError,
		},
		{
			desc:       "no key",
			value:      "value",
			checkError: require.Error,
		},
		{
			desc:       "start with number",
			key:        "1234key",
			checkError: require.Error,
		},
		{
			desc:       "start with space",
			key:        " key",
			checkError: require.Error,
		},
		{
			desc:       "contains '-'",
			key:        "key-name",
			checkError: require.Error,
		},
	}

	for _, testCase := range testCases {
		err := writer.validateValue(testCase.key, testCase.value)
		testCase.checkError(t, err)
	}
}
