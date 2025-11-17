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

package keyid

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	provider := NewProvider()
	assert.NotNil(t, provider)
}

func TestName(t *testing.T) {
	assert.NotEmpty(t, NewProvider().Name())
}

func TestCommand(t *testing.T) {
	tests := []struct {
		name         string
		provider     *Provider
		providedArgs []string
		expectedArgs []string
	}{
		{
			name:     "no config, no existing args",
			provider: NewProvider(),
		},
		{
			name:         "no config, existing args",
			provider:     NewProvider(),
			providedArgs: []string{"arg1", "arg2"},
			expectedArgs: []string{"arg1", "arg2"},
		},
		{
			name:         "config, no existing args",
			provider:     NewProvider(WithKeyID("keyID"), WithGPGHomeDir("gpgHomeDir")),
			expectedArgs: []string{"--gpg-home-dir", "gpgHomeDir", "--key-id", "keyID"},
		},
		{
			name:         "config, existing args",
			provider:     NewProvider(WithKeyID("keyID"), WithGPGHomeDir("gpgHomeDir")),
			providedArgs: []string{"arg", "filename"},
			expectedArgs: []string{"arg", "--gpg-home-dir", "gpgHomeDir", "--key-id", "keyID", "filename"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &exec.Cmd{
				Args: tt.providedArgs,
			}

			err := tt.provider.Command(t.Context(), cmd)
			require.NoError(t, err)

			if len(tt.expectedArgs) == 0 {
				// Nil or zero length, doesn't matter which
				assert.Empty(t, cmd.Args)
			} else {
				assert.Equal(t, tt.expectedArgs, cmd.Args)
			}
		})
	}
}

func TestClose(t *testing.T) {
	assert.NoError(t, NewProvider().Close(t.Context()))
}
