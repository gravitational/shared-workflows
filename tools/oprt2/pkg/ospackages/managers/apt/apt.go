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

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
)

// APT is a [ospackages.Manager] that manages Debian files in APT repos.
type APT struct {
	// Key is component name, value is a set of matchers to select files that should be associated with each component
	components map[string][]*regexp.Regexp
	// Key is repo name (e.g. `ubuntu`, `debian` for Gravitational), value is APT distros that should be associated with each repo (e.g. `noble`, `trixie` for Gravitational)
	repos       map[string][]string
	fileManager filemanager.FileManager
	publisher   ospackages.APTPublisher
	logger      *slog.Logger
}

var _ ospackages.Manager = (*APT)(nil)

// NewAPT creates a new APT package manager instance.
func NewAPT(fileManager filemanager.FileManager, opts ...APTOption) *APT {
	apt := &APT{
		logger: logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(apt)
	}

	return apt
}

// GetPackagePublishingTasks returns tasks for publishing packages.
func (apt *APT) GetPackagePublishingTasks(ctx context.Context) ([]ospackages.PackagePublishingTask, error) {
	// Collect possible files to upload
	candidateItems, err := apt.fileManager.ListItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get packages to publish to APT repos: %w", err)
	}

	// Map the components to files components
	// Given a map of components with regex file selectors for each component, get a specific list of files that
	// should be associated with each component.
	// For example, given {"noble": ["teleport-(amd|arm)64\.deb", other-packages\.deb]}, produce
	// {"noble": ["teleport-amd64.deb", "teleport-arm64.deb", "other-packages.deb (if exists)"]}
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

	// Enqueue an upload of the files
	publishingTasks := make([]ospackages.PackagePublishingTask, 0)
	for component, packageFilePaths := range componentFiles {
		for distro, distroVersions := range apt.repos {
			for _, distroVersion := range distroVersions {
				for _, packageFilePath := range packageFilePaths {
					publishFunc := func(ctx context.Context) error {
						if err := apt.publisher.PublishToAPTRepo(ctx, distro, distroVersion, component, packageFilePath); err != nil {
							return fmt.Errorf(
								"failed to publish package at %q for distro %s/%s %s via %s: %w",
								packageFilePath,
								distro,
								distroVersion,
								component,
								apt.publisher.Name(),
								err,
							)
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
	return nil
}
