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
type APTOption func(apt *Manager)

// Repos is the full config for what APT repos that should be used.
// First-level key is repo name (e.g. `ubuntu`, `debian` for Gravitational).
// Second-level key is distribution (e.g. `plucky`, `trixie` for Gravitational).
// Third-level key is component (e.g. `stable/rolling`, `stable/v18` for Gravitational).
// Values are a list of regular expressions that match file paths that should be associated with
// with repo/distribution/component combination.
// See https://wiki.debian.org/DebianRepository/Format#Overview for details.
// Note: This will overwrite existing repos, not replace them. Providing `WithRepos`
// multiple times is not supported.
func WithRepos(repos map[string]map[string]map[string][]*regexp.Regexp) APTOption {
	return func(apt *Manager) {
		for repoName, distributions := range repos {
			for distributionName, components := range distributions {
				for componentName, fileNameMatchers := range components {
					// Get unique values file matcher values only
					uniqueMatchers := make(map[string]*regexp.Regexp, len(fileNameMatchers))
					for _, fileNameMatcher := range fileNameMatchers {
						uniqueMatchers[fileNameMatcher.String()] = fileNameMatcher
					}

					if len(uniqueMatchers) == len(fileNameMatchers) {
						// Short-circuit when all items are already unique
						continue
					}

					keys := slices.Collect(maps.Keys(uniqueMatchers))
					slices.Sort(keys)

					sortedUniqueMatchers := make([]*regexp.Regexp, 0, len(keys))
					for _, key := range keys {
						sortedUniqueMatchers = append(sortedUniqueMatchers, uniqueMatchers[key])
					}

					components[componentName] = sortedUniqueMatchers
				}

				distributions[distributionName] = components
			}

			repos[repoName] = distributions
		}

		apt.repos = repos
	}
}

// WithLogger configures the package manager with the provided logger.
func WithLogger(logger *slog.Logger) APTOption {
	return func(apt *Manager) {
		if logger == nil {
			logger = logging.DiscardLogger
		}
		apt.logger = logger
	}
}

// WithPublisher sets the publisher to use.
func WithPublisher(publisher ospackages.APTPublisher) APTOption {
	return func(apt *Manager) {
		if publisher == nil {
			publisher = discard.DiscardPublisher
		}

		apt.publisher = publisher
	}
}
