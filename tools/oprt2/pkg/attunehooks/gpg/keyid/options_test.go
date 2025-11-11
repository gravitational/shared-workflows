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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithGPGHomeDir(t *testing.T) {
	provider := &Provider{}
	gpgHomeDir := "home/dir/path"

	opt := WithGPGHomeDir(gpgHomeDir)
	opt(provider)

	assert.Equal(t, gpgHomeDir, provider.gpgHomeDir)
}

func TestWithKeyID(t *testing.T) {
	provider := &Provider{}
	keyID := "test key ID"

	opt := WithKeyID(keyID)
	opt(provider)

	assert.Equal(t, keyID, provider.keyID)
}
