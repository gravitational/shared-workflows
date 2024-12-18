package values

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	hashedExpectedString := "<redacted $2a$10$"

	testCases := []struct {
		value                  Value
		expectedStringContents string
	}{
		{
			value:                  Value{UnderlyingValue: "public value"},
			expectedStringContents: "public value",
		},
		{
			value:                  Value{UnderlyingValue: "secret value", ShouldMask: true},
			expectedStringContents: hashedExpectedString,
		},
		{
			// Arbitrary string at length limit for bcrypt
			value:                  Value{UnderlyingValue: strings.Repeat("a", 72), ShouldMask: true},
			expectedStringContents: hashedExpectedString,
		},
		{
			// Arbitrary long string above len limit
			value:                  Value{UnderlyingValue: strings.Repeat("a", 73), ShouldMask: true},
			expectedStringContents: hashedExpectedString,
		},
	}

	for _, testCase := range testCases {
		renderedString := testCase.value.String()
		require.Contains(t, renderedString, testCase.expectedStringContents)

		if testCase.value.ShouldMask {
			require.NotContains(t, renderedString, testCase.value.UnderlyingValue)
		}
	}
}
