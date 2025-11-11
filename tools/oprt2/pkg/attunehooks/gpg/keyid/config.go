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

import "github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"

// FromConfig creates a new Provider instance from the provided config.
func FromConfig(config *config.GPGKeyIDProvider) *Provider {
	opts := make([]ProviderOption, 0, 2)

	if config.GPGHomeDirectory != nil {
		opts = append(opts, WithGPGHomeDir(*config.GPGHomeDirectory))
	}

	if config.KeyID != nil {
		opts = append(opts, WithKeyID(*config.KeyID))
	}

	return NewProvider(opts...)
}
