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
	"log/slog"
	"maps"
	"regexp"
	"slices"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages/publishers/discard"
)

// APTOption provides optional configuration to the APT package manager.
type APTOption func(apt *APT)

// WithComponents configures the package manager to map files to APT components. Key is
// the component name, value is a list of regular expressions used to match files to the
// component. Each expression will be matched against every file in the storage backend,
// including the file's path. For example, `some/path/teleport.deb`.
// Note: This will overwrite existing components, not replace them. Providing
// `WithComponents` multiple times is not supported.
func WithComponents(components map[string][]*regexp.Regexp) APTOption {
	return func(apt *APT) {
		apt.components = components

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
// is the distro name (e.g. ubuntu, debian), value is distro version (e.g. noble, plucky).
// Note: This will overwrite existing distros, not replace them. Providing `WithDistros`
// multiple times is not supported.
func WithDistros(distros map[string][]string) APTOption {
	return func(apt *APT) {
		apt.distros = distros

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

// WithPublisher sets the publisher to use.
func WithPublisher(publisher ospackages.APTPublisher) APTOption {
	return func(apt *APT) {
		if publisher == nil {
			publisher = discard.NewDiscardPublisher()
		}

		apt.publisher = publisher
	}
}
