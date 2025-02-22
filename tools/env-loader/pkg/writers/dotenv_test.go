package writers

import (
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

func TestDotenvValidation(t *testing.T) {
	writer := NewDotenvWriter()
	testCases := []struct {
		desc       string
		key        string
		value      values.Value
		checkError require.ErrorAssertionFunc
	}{
		{
			desc:  "valid value",
			key:   "key",
			value: values.Value{UnderlyingValue: "value"},
		},
		{
			desc: "no value",
			key:  "key",
		},
		{
			desc: "contains '_'",
			key:  "key_name",
		},
		{
			desc: "all caps",
			key:  "KEY",
		},
		{
			desc:       "no key",
			value:      values.Value{UnderlyingValue: "value"},
			checkError: require.Error,
		},
		{
			desc:       "no key, secret value",
			value:      values.Value{UnderlyingValue: "secret value", ShouldMask: true},
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

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}
		testCase.checkError(t, err)

		if err != nil && testCase.value.ShouldMask {
			require.NotContains(t, err.Error(), testCase.value.UnderlyingValue)
		}
	}
}

func TestDotenvFormat(t *testing.T) {
	testCases := []struct {
		desc       string
		values     map[string]values.Value
		canBeEmpty bool
		checkError require.ErrorAssertionFunc
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
			canBeEmpty: true,
		},
		{
			desc:       "no values",
			canBeEmpty: true,
		},
		{
			desc: "empty key",
			values: map[string]values.Value{
				"": {UnderlyingValue: "value"},
			},
			checkError: require.Error,
			canBeEmpty: true,
		},
	}

	writer := NewDotenvWriter()
	for _, testCase := range testCases {
		formattedStr, err := writer.FormatEnvironmentValues(testCase.values)

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		testCase.checkError(t, err, "writer failed with test case %q", testCase.desc)
		if !testCase.canBeEmpty {
			require.NotEmpty(t, formattedStr, "writer output is empty")
		}
	}
}
