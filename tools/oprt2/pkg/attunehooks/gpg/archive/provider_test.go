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

package archive

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkdir -p /tmp/testempty && pushd /tmp/testempty && tar -czf - . | base64 -w 0 && popd > /dev/null && rm -rf /tmp/testempty
const emptyDirArchive = "H4sIAAAAAAAAA+3OMQ7CQAwEQD/lXkDuQsy9BwE1Egm8n1Ag0SCqUM00K3m38G6IzdVVz3xl61k/8y1atjr1Oo7Tem8194couf1rEfd5Od5Kicd8up4v33e/egAAAAAAAAAA"

// mkdir -p /tmp/testempty && pushd /tmp/testempty && echo "test file" > testfile && tar -czf - . | base64 -w 0 && popd >/dev/null && rm -rf /tmp/testempty
const testFileArchive = "H4sIAAAAAAAAA+3RQQrCMBBA0Vl7ipygncRMcx7RCgVBMNHzaxZiKRRxEUT8bzOLzGLC73ppTh+SWZ0+mc7nk3jzGlNIQQdRrxaTOGt/msg1l93FObnl/fkwru+9e/9RXV/GXI7TqeHXauAhxvX+Piz6h60GcdrupJc/71/ru5p/8+1LAAAAAAAAAAAAAAAAAHzqDqNq3A4AKAAA"

func TestNewProvider(t *testing.T) {
	t.Run("with no archive", func(t *testing.T) {
		_, err := NewProvider(t.Context(), "")
		assert.Error(t, err)
	})

	t.Run("with invalid b64 string", func(t *testing.T) {
		_, err := NewProvider(t.Context(), "abc123 not base64-encoded string")
		assert.Error(t, err)
	})

	t.Run("with invalid tarball", func(t *testing.T) {
		archive := base64.StdEncoding.EncodeToString([]byte("abc123 not a tarball"))
		_, err := NewProvider(t.Context(), archive)
		assert.Error(t, err)
	})

	t.Run("with valid but empty tarball", func(t *testing.T) {
		provider, err := NewProvider(t.Context(), emptyDirArchive)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, provider.Close(context.TODO())) })

		assert.DirExists(t, provider.credentialsDirectory)
		dirEntries, err := os.ReadDir(provider.credentialsDirectory)
		assert.NoError(t, err)
		assert.Empty(t, dirEntries)
	})

	t.Run("with single test file tarball", func(t *testing.T) {
		provider, err := NewProvider(t.Context(), testFileArchive)
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, provider.Close(context.TODO())) })

		testFilePath := filepath.Join(provider.credentialsDirectory, "testfile")
		require.FileExists(t, testFilePath)

		contents, err := os.ReadFile(testFilePath)
		require.NoError(t, err)
		assert.Equal(t, "test file\n", string(contents))
	})
}

func TestCommand(t *testing.T) {
	tests := []struct {
		name         string
		provider     *Provider
		providedArgs []string
		expectedArgs []string
	}{
		{
			name: "minimal config, no existing args",
			provider: &Provider{
				credentialsDirectory: "test-cred-dir",
			},
			expectedArgs: []string{"--gpg-home-dir", "test-cred-dir"},
		},
		{
			name: "minimal config, existing args",
			provider: &Provider{
				credentialsDirectory: "test-cred-dir",
			},
			providedArgs: []string{"arg1", "arg2"},
			expectedArgs: []string{"arg1", "--gpg-home-dir", "test-cred-dir", "arg2"},
		},
		{
			name: "with key ID, no existing args",
			provider: &Provider{
				credentialsDirectory: "test-cred-dir",
				keyID:                "keyID",
			},
			expectedArgs: []string{"--gpg-home-dir", "test-cred-dir", "--key-id", "keyID"},
		},
		{
			name: "with key ID, existing args",
			provider: &Provider{
				credentialsDirectory: "test-cred-dir",
				keyID:                "keyID",
			},
			providedArgs: []string{"arg", "filename"},
			expectedArgs: []string{"arg", "--gpg-home-dir", "test-cred-dir", "--key-id", "keyID", "filename"},
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
