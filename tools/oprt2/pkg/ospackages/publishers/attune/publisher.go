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

package attune

import (
	"context"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
)

// Publisher is a publisher that publishes via Attune.
type Publisher struct {
	logger *slog.Logger
	attune commandrunner.Runner
}

var _ ospackages.Publisher = (*Publisher)(nil)

// NewPublisher creates a new publisher that uses Attune to upload packages.
func NewPublisher(attuneRunner commandrunner.Runner, opts ...PublisherOpt) *Publisher {
	ap := &Publisher{
		logger: logging.DiscardLogger,
		attune: attuneRunner,
	}

	for _, opt := range opts {
		opt(ap)
	}

	return ap
}

// Name is the name of the package publisher.
func (*Publisher) Name() string {
	return "Attune"
}

// PublishToAPTRepo publishes the package at the provided file path to the publisher's APT repo,
// with the set distro, and component.
func (ap *Publisher) PublishToAPTRepo(ctx context.Context, repo, distro, component, packageFilePath string) error {
	ap.logger.DebugContext(ctx, "publishing package via Attune",
		"repo", repo,
		"distro", distro,
		"component", component,
		"packageFilePath", packageFilePath,
	)

	// Attune performs some deduping magic, so after a package is uploaded once,
	// packages will not be uploaded again. This greatly reduces publishing time.
	// If the first distro versions seem to take a long time to publish, this is why. The first
	// pass associated with them do substantially more work then every iteration after them.
	return ap.attune.Run(ctx, "attune", "apt", "package", "add",
		"--repo", repo,
		"--distribution", distro,
		"--component", component,
		packageFilePath,
	)
}

// Close closes the publisher.
func (ap *Publisher) Close(ctx context.Context) error {
	return nil
}
