package notarize

import (
	"errors"
	"testing"

	"github.com/zeebo/assert"
)

func TestSubmit_retries(t *testing.T) {
	tests := []struct {
		name           string
		failedAttempts int
		wantErr        bool
		maxRetries     int
	}{
		{
			name:           "succeed first try",
			failedAttempts: 0,
			wantErr:        false,
			maxRetries:     5,
		},
		{
			name:           "fails after maximum tries",
			failedAttempts: 10,
			wantErr:        true,
			maxRetries:     5,
		},
		{
			name:           "no retries - succeess",
			failedAttempts: 0,
			wantErr:        false,
			maxRetries:     0,
		},
		{
			name:           "no retries - failure",
			failedAttempts: 1,
			wantErr:        true,
			maxRetries:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &cmdCounter{
				failedAttempts: tt.failedAttempts,
			}
			tool := NewTool(
				Creds{
					AppleUsername:   "FAKE_USERNAME",
					ApplePassword:   "FAKE_PASSWORD",
					SigningIdentity: "FAKE_IDENTITY",
					BundleID:        "FAKE_BUNDLE_ID",
				},
				ToolOpts{
					Retry: tt.maxRetries,
				},
			)
			tool.cmdRunner = counter
			_, err := tool.Submit("fake/package.zip")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.failedAttempts+1, counter.count)
		})
	}
}

type cmdCounter struct {
	// internal counter
	count int

	// Return an error for a number of attempts.
	failedAttempts int
}

func (c *cmdCounter) RunCommand(path string, args ...string) (string, error) {
	c.count += 1
	if c.count > c.failedAttempts {
		return "{\"id\": \"0\"}", nil
	}
	return "", errors.New("failed")
}
