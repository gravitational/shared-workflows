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
	"slices"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages/publishers/discard"
)

// Manager is a [ospackages.Manager] that manages Debian files in Manager repos.
type Manager struct {
	fileManager filemanager.FileManager
	publisher   ospackages.APTPublisher
	logger      *slog.Logger

	// Repos is the full config for what APT repos that should be used.
	// First-level key is repo name (e.g. `ubuntu`, `debian` for Gravitational).
	// Second-level key is distribution (e.g. `plucky`, `trixie` for Gravitational).
	// Third-level key is component (e.g. `stable/rolling`, `stable/v18` for Gravitational).
	// Values are a list of regular expressions that match file paths that should be associated with
	// with repo/distribution/component combination.
	// See https://wiki.debian.org/DebianRepository/Format#Overview for details.
	repos map[string]map[string]map[string][]*regexp.Regexp
}

var _ ospackages.Manager = (*Manager)(nil)

// NewManager creates a new APT package manager instance.
func NewManager(fileManager filemanager.FileManager, opts ...APTOption) *Manager {
	apt := &Manager{
		fileManager: fileManager,
		publisher:   discard.DiscardPublisher,
		logger:      logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(apt)
	}

	return apt
}

// GetPackagePublishingTasks returns tasks for publishing packages.
func (apt *Manager) GetPackagePublishingTasks(ctx context.Context) ([]ospackages.PackagePublishingTask, error) {
	// Collect files to upload
	repoFiles, totalPackageCount, err := apt.getRepoFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get repo files: %w", err)
	}

	// Enqueue an upload of the files
	publishingTasks := make([]ospackages.PackagePublishingTask, 0, totalPackageCount)
	for repoName, distributions := range repoFiles {
		for distributionName, components := range distributions {
			for componentName, packageFilePaths := range components {
				for _, packageFilePath := range packageFilePaths {
					publishFunc := func(ctx context.Context) error {
						if err := apt.publisher.PublishToAPTRepo(ctx, repoName, distributionName, componentName, packageFilePath); err != nil {
							return fmt.Errorf(
								"failed to publish package at %q for %s %s %s via %s: %w",
								packageFilePath,
								repoName,
								distributionName,
								componentName,
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

// getRepoFiles finds matching files for each repo's file selectors.
// Returns the same structure as apt.repo, but with actual file names instead of regular expressions.
// Also returns the total number of packages across all components.
func (apt *Manager) getRepoFiles(ctx context.Context) (map[string]map[string]map[string][]string, int, error) {
	// Collect possible files to upload
	candidateItems, err := apt.fileManager.ListItems(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get packages for APT repos: %w", err)
	}

	repoFiles := make(map[string]map[string]map[string][]string, len(apt.repos))
	packageCount := 0

	for repoName, distributions := range apt.repos {
		distributionFiles := make(map[string]map[string][]string, len(distributions))

		for distributionName, components := range distributions {
			componentFiles := make(map[string][]string, len(components))

			for componentName, fileMatchers := range components {
				// Assume that each file matcher should match at least one file
				matchingFiles := make([]string, 0, len(fileMatchers))

				for _, fileMatcher := range fileMatchers {
					for _, candidateItem := range candidateItems {
						if !fileMatcher.MatchString(candidateItem) {
							continue
						}

						apt.logger.DebugContext(ctx, "found matching file", "repo", repoName, "distribution", distributionName, "component", componentName, "file", candidateItem)
						localCandidateFilePath, err := apt.fileManager.GetLocalFilePath(ctx, candidateItem)
						if err != nil {
							return nil, 0, fmt.Errorf("failed to get local file path to %q via file manager %q: %q", candidateItem, apt.fileManager.Name(), err)
						}

						matchingFiles = append(matchingFiles, localCandidateFilePath)
					}
				}

				// Return only unique items
				slices.Sort(matchingFiles)
				matchingFiles = slices.Compact(matchingFiles)

				componentFiles[componentName] = matchingFiles
				packageCount += len(matchingFiles)
			}

			distributionFiles[distributionName] = componentFiles
		}

		repoFiles[repoName] = distributionFiles
	}

	return repoFiles, packageCount, nil
}

// Name is the name of the package manager.
func (apt *Manager) Name() string {
	return "APT"
}

// Close closes the package manager
func (apt *Manager) Close(ctx context.Context) error {
	return nil
}
