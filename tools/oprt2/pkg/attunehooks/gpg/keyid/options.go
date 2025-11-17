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

// ProviderOption provides optional configuration to the Provider.
type ProviderOption func(p *Provider)

// WithGPGHomeDir specifies the directory containing the GPG keychain.
func WithGPGHomeDir(gpgHomeDir string) ProviderOption {
	return func(p *Provider) {
		p.gpgHomeDir = gpgHomeDir
	}
}

// WithKeyID sets the key ID to provide Attune. If not set and only one key exists in the
// directory, Attune will attempt to use it. Required when multiple keys exist in the directory.
func WithKeyID(keyID string) ProviderOption {
	return func(p *Provider) {
		p.keyID = keyID
	}
}
