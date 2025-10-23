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
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager/apt"
)

// FromConfig builds a new package manager from the provided config, adding any provided authentication hooks.
func FromConfig(ctx context.Context, config config.PackageManager, logger *slog.Logger, attuneHooks ...commandrunner.Hook) (packagemanager.Manager, error) {
	switch {
	case config.APT != nil:
		return apt.FromConfig(ctx, *config.APT, logger, attuneHooks...)
	default:
		return nil, fmt.Errorf("no package manager config provided")
	}
}
