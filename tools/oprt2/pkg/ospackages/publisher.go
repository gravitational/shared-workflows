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

// Publisher handles the publishing of manager-specific packages.
// This should be embedded in manager-specific publishers.
type Publisher interface {
	// Name is the name of the package publisher.
	Name() string

	// Close closes the publisher
	Close(ctx context.Context) error
}

type APTPublisher interface {
	Publisher

	// PublishToAPTRepo publishes the package at the provided file path to the publisher's APT repo,
	// with the set distro, and component.
	PublishToAPTRepo(ctx context.Context, repo, distro, component, packageFilePath string) error
}
