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
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// FromConfig creates a new Provider instance from the provided config.
func FromConfig(ctx context.Context, config *config.GPGArchiveProvider, logger *slog.Logger) (*Provider, error) {
	opts := make([]ProviderOption, 0, 2)

	if config.KeyID != nil {
		opts = append(opts, WithKeyID(*config.KeyID))
	}

	opts = append(opts, WithLogger(logger))

	provider, err := NewProvider(ctx, config.Archive, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GPG archive provider: %w", err)
	}

	return provider, nil
}
