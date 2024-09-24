package git

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestampForRef(t *testing.T) {
	// Mostly just a sanity test to prove that time parsing actually works
	var tests = []struct {
		name         string
		gitTimestamp string
	}{
		{
			name:         "ISO 8601 with offset",
			gitTimestamp: "2024-09-10T13:05:11-05:00",
		},
		{
			name:         "ISO 8601 in UTC",
			gitTimestamp: "2024-09-10T13:05:11Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &Repo{
				runner: &fakeRunner{
					t:        t,
					toStdout: tt.gitTimestamp,
				},
			}
			_, err := repo.TimestampForRef("")
			assert.NoError(t, err)
		})
	}
}

type fakeRunner struct {
	t        *testing.T
	toStdout string
}

func (r *fakeRunner) Run(cmd *exec.Cmd) error {
	_, err := fmt.Fprint(cmd.Stdout, r.toStdout)
	require.NoError(r.t, err)
	return nil
}
