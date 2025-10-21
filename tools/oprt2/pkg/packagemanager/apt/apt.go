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
	"maps"
	"regexp"
	"slices"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager"
)

// APT is a [packagemanager.Manager] that manages Debian files to APT repos.
type APT struct {
	components   map[string][]*regexp.Regexp
	distros      map[string][]string
	fm           filemanager.FileManager
	hooks        []commandrunner.Hook
	attuneRunner *commandrunner.Runner
	logger       *slog.Logger
}

var _ packagemanager.Manager = (*APT)(nil)

// APTOption provides optional configuration to the APT package manager.
type APTOption func(apt *APT)

// WithAttuneHooks adds hooks to the Attune command runner.
func WithAttuneHooks(hooks ...commandrunner.Hook) APTOption {
	return func(apt *APT) {
		apt.hooks = append(apt.hooks, hooks...)
	}
}

// WithComponents configures the package manager to map files to APT components. Key is
// the component name, value is a list of regular expressions used to match files to the
// component. Each expression will be matched against every file in the storage backend,
// including the file's path. For example, `some/path/teleport.deb`.
func WithComponents(components map[string][]*regexp.Regexp) APTOption {
	return func(apt *APT) {
		if len(apt.components) == 0 {
			apt.components = make(map[string][]*regexp.Regexp, len(components))
		}

		// Deep merge the maps
		for componentName, fileNameMatchers := range components {
			if _, ok := apt.components[componentName]; !ok {
				components[componentName] = fileNameMatchers
				continue
			}
			components[componentName] = append(apt.components[componentName], fileNameMatchers...)
		}

		// Get unique matchers
		// This does not preserve order, but it shouldn't need to
		for componentName, fileNameMatchers := range components {
			uniqueMatchers := make(map[string]*regexp.Regexp, len(fileNameMatchers))
			for _, fileNameMatcher := range fileNameMatchers {
				uniqueMatchers[fileNameMatcher.String()] = fileNameMatcher
			}

			if len(uniqueMatchers) != len(fileNameMatchers) {
				keys := slices.Collect(maps.Keys(uniqueMatchers))
				slices.Sort(keys)

				sortedUniqueMatchers := make([]*regexp.Regexp, 0, len(keys))
				for _, key := range keys {
					sortedUniqueMatchers = append(sortedUniqueMatchers, uniqueMatchers[key])
				}

				components[componentName] = sortedUniqueMatchers
			}
		}

		apt.components = components
	}
}

// WithDistros configures the package manager to add packages to the specified distros. Key
// is the distro name (e.g. ubuntu, debian), value is distro version (e.g. noble, plucky)
func WithDistros(distros map[string][]string) APTOption {
	return func(apt *APT) {
		if len(apt.distros) == 0 {
			apt.distros = make(map[string][]string, len(apt.distros))
		}

		// Deep merge the maps
		for distroName, distroVersion := range distros {
			if _, ok := apt.distros[distroName]; !ok {
				distros[distroName] = distroVersion
				continue
			}
			distros[distroName] = append(apt.distros[distroName], distroVersion...)
		}

		// Get unique values only
		for distroName, distroVersions := range distros {
			uniqueVersions := make(map[string]struct{}, len(distroVersions))
			for _, distroVersion := range distroVersions {
				uniqueVersions[distroVersion] = struct{}{}
			}

			// Collect the results if anything has changed
			if len(uniqueVersions) != len(distroVersions) {
				distros[distroName] = slices.Collect(maps.Keys(uniqueVersions))
			}

			// Sort to make the process a little more predictable (e.g. log output ordering)
			slices.Sort(distros[distroName])
		}

		apt.distros = distros
	}
}

// WithLogger configures the package manager with the provided logger.
func WithLogger(logger *slog.Logger) APTOption {
	return func(apt *APT) {
		if logger == nil {
			logger = logging.DiscardLogger
		}
		apt.logger = logger
	}
}

// NewAPT creates a new APT package manager instance.
func NewAPT(fm filemanager.FileManager, opts ...APTOption) *APT {
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
	candidateItems, err := apt.fm.ListItems(ctx)
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
				localCandidateFilePath, err := apt.fm.GetLocalFilePath(ctx, candidateItem)
				if err != nil {
					return nil, fmt.Errorf("failed to get local file path to %q via file manager %q: %q", candidateItem, apt.fm.Name(), err)
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
