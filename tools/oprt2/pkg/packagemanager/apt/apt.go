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

package apt

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager"
)

// APT is a [packagemanager.Manager] that manages Debian files in APT repos.
type APT struct {
	components   map[string][]*regexp.Regexp
	distros      map[string][]string
	fileManager  filemanager.FileManager
	hooks        []commandrunner.Hook
	attuneRunner *commandrunner.Runner
	logger       *slog.Logger
}

var _ packagemanager.Manager = (*APT)(nil)

// NewAPT creates a new APT package manager instance.
func NewAPT(fileManager filemanager.FileManager, opts ...APTOption) *APT {
	apt := &APT{
		logger: logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(apt)
	}

	apt.attuneRunner = commandrunner.NewRunner(commandrunner.WithHooks(apt.hooks...))

	return apt
}

// GetPackagePublishingTasks returns tasks for publishing packages.
func (apt *APT) GetPackagePublishingTasks(ctx context.Context) ([]packagemanager.PackagePublishingTask, error) {
	// Collect possible files to upload
	candidateItems, err := apt.fileManager.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get packages to publish to APT repos: %w", err)
	}

	// Map the files to specific components
	componentFiles := make(map[string][]string, len(apt.components))
	for component, filePatterns := range apt.components {
		for _, candidateItem := range candidateItems {
			for _, filePattern := range filePatterns {
				if !filePattern.MatchString(candidateItem) {
					continue
				}

				apt.logger.DebugContext(ctx, "queuing file for publishing", "file", candidateItem)
				localCandidateFilePath, err := apt.fileManager.GetLocalFilePath(ctx, candidateItem)
				if err != nil {
					return nil, fmt.Errorf("failed to get local file path to %q via file manager %q: %q", candidateItem, apt.fileManager.Name(), err)
				}

				componentFiles[component] = append(componentFiles[component], localCandidateFilePath)
				break
			}
		}
	}

	// Build the command runner, call and defer hooks
	if err := apt.attuneRunner.Setup(ctx); err != nil {
		return nil, fmt.Errorf("attune runner setup hook failed: %w", err)
	}

	// Enqueue an upload of the files
	publishingTasks := make([]packagemanager.PackagePublishingTask, 0)
	for component, packageFilePaths := range componentFiles {
		for distro, distroVersions := range apt.distros {
			for _, distroVersion := range distroVersions {
				// Attune performs some deduping magic, so after this loop is iterated over once,
				// packages will not be uploaded again. This greatly reduces publishing time.
				// If the first distro versions seem to take a long time to publish, this is why. The loop
				// iterations associated with them do substantially more work then every iteration after
				// them.
				for _, packageFilePath := range packageFilePaths {
					// Publish with Attune
					publishFunc := func(ctx context.Context) error {
						err := apt.attuneRunner.Run(ctx, "attune", "apt", "package", "add",
							"--repo", distro,
							"--distribution", distroVersion,
							"--component", component,
							packageFilePath,
						)

						if err != nil {
							return fmt.Errorf("attune package publishing failed: %w", err)
						}

						return nil
					}

					publishingTasks = append(publishingTasks, publishFunc)
				}
			}
		}
	}

	return publishingTasks, nil
}

// Name is the name of the package manager.
func (apt *APT) Name() string {
	return "APT"
}

// Close closes the package manager
func (apt *APT) Close(ctx context.Context) error {
	if err := apt.attuneRunner.Close(ctx); err != nil {
		return fmt.Errorf("attune runner cleanup failed: %w", err)
	}

	return nil
}
