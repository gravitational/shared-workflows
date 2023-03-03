package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractMajorVersionFromTagName(t *testing.T) {

	testCases := []struct {
		description        string
		in                 string
		expected           int
		expectErrSubstring string
	}{
		{
			description: "Standard",
			in:          "v12.0.4",
			expected:    12,
		},
		{
			description: "Standard with label",
			in:          "v12.0.0-passwordless-windows",
			expected:    12,
		},
		{
			description: "Standard with label and number",
			in:          "v12.0.0-alpha.2",
			expected:    12,
		},
		{
			description:        "Number without semantic versioning",
			in:                 "12",
			expected:           0,
			expectErrSubstring: "must be formatted",
		},
		{
			description:        "Semantic versioning without the v",
			in:                 "12.0.0",
			expected:           0,
			expectErrSubstring: "must be formatted",
		},
	}

	for _, c := range testCases {
		t.Run(c.description, func(t *testing.T) {
			i, err := extractMajorVersionFromTagName(c.in)
			if c.expectErrSubstring == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, c.expectErrSubstring)
			}
			assert.Equal(t, c.expected, i)
		})
	}

}
