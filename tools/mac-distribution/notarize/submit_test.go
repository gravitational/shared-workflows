/*
 *  Copyright 2025 Gravitational, Inc
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

package notarize

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
			tool, err := NewTool(
				Creds{
					AppleUsername:   "FAKE_USERNAME",
					ApplePassword:   "FAKE_PASSWORD",
					SigningIdentity: "FAKE_IDENTITY",
					TeamID:          "FAKE_TEAM_ID",
				},
				MaxRetries(tt.maxRetries),
			)
			assert.NoError(t, err)
			tool.cmdRunner = counter
			_, err = tool.Submit("fake/package.zip")
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

func (c *cmdCounter) RunCommand(path string, args ...string) ([]byte, error) {
	c.count += 1
	if c.count > c.failedAttempts {
		return []byte("{\"id\": \"0\"}"), nil
	}
	return nil, errors.New("failed")
}
