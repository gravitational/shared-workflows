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

package loader

import (
	"context"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg/archive"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg/keyid"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// FromConfig builds a new GPG provider from the provided config.
func FromConfig(ctx context.Context, config *config.GPGProvider, logger *slog.Logger) (commandrunner.Hook, error) {
	switch {
	case config.Archive != nil:
		return archive.FromConfig(ctx, config.Archive, logger)
	case config.KeyID != nil:
		return keyid.FromConfig(config.KeyID), nil
	}

	return nil, nil
}
