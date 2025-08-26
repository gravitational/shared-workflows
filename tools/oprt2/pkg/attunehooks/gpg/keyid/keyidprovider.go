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
	"context"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
)

// KeyIDProvider is a [commandrunner.Hook] that provides access to a GPG key via a key
// that has already been loaded into the GPG home directory.
type KeyIDProvider struct {
	keyID      string
	gpgHomeDir string
}

var _ commandrunner.PreCommandHook = &KeyIDProvider{}

type KeyIDProviderOption func(kidp *KeyIDProvider)

// WithGPGHomeDir specifies the directory containing the GPG keychain.
func WithGPGHomeDir(gpgHomeDir string) KeyIDProviderOption {
	return func(kidp *KeyIDProvider) {
		kidp.gpgHomeDir = gpgHomeDir
	}
}

// WithKeyID sets the key ID to provide Attune. If not set and only one key exists in the
// directory, Attune will attempt to use it. Required when multiple keys exist in the directory.
func WithKeyID(keyID string) KeyIDProviderOption {
	return func(kidp *KeyIDProvider) {
		kidp.keyID = keyID
	}
}

func NewKeyIDProvider(opts ...KeyIDProviderOption) *KeyIDProvider {
	kipd := &KeyIDProvider{}

	for _, opt := range opts {
		opt(kipd)
	}

	return kipd
}

func (kidp *KeyIDProvider) Name() string {
	return "GPG key ID provider"
}

func (kidp *KeyIDProvider) PreCommand(_ context.Context, _ *string, args *[]string) error {
	kidpArgs := make([]string, 0, 4)

	if kidp.gpgHomeDir != "" {
		kidpArgs = append(kidpArgs, "--gpg-home-dir", kidp.gpgHomeDir)
	}

	if kidp.keyID != "" {
		kidpArgs = append(kidpArgs, "--key-id", kidp.keyID)
	}

	*args = append(kidpArgs, *args...)

	return nil
}
