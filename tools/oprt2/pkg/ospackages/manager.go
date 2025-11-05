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

package ospackages

import (
	"context"
)

type PackagePublishingTask func(context.Context) error

// Manager handles the management (publishing, pulling) of all configured packages.
type Manager interface {
	// GetPackagePublishingTasks returns tasks for publishing packages.
	GetPackagePublishingTasks(ctx context.Context) ([]PackagePublishingTask, error)

	// Name is the name of the package manager.
	Name() string

	// Close closes the package manager
	Close(ctx context.Context) error
}
