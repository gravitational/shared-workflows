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

package packagemanager

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// PackageManager handles the publishing of all configured packages.
type PackageManager interface {
	// EnqueueForPublishing adds tasks to the error group for publishing packages.
	EnqueueForPublishing(ctx context.Context, queue *errgroup.Group) error

	// Name is the name of the package manager.
	Name() string
}

// ClosablePackageManager is a [PackageManager] that can be closed.
type ClosablePackageManager interface {
	PackageManager
	Close(ctx context.Context) error
}
