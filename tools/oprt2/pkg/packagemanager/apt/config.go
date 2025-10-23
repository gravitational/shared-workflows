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

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
)

// FromConfig creates a new APT instance from the provided config, and optional Attune hooks.
func FromConfig(ctx context.Context, config config.APT, logger *slog.Logger, attuneHooks ...commandrunner.Hook) (*APT, error) {
	fileManager, err := filemanager.FromConfig(ctx, config.FileSource)
	if err != nil {
		return nil, fmt.Errorf("failed to create file manager from config: %w", err)
	}

	components, err := getAPTComponentsFromConfig(config.Components)
	if err != nil {
		return nil, fmt.Errorf("failed to get APT components: %w", err)
	}

	// This must done last to avoid leaking the provider if something else fails
	gpgProvider := gpg.FromConfig(config.GPG)

	apt := NewAPT(
		fileManager,
		WithLogger(logger),
		WithAttuneHooks(append(attuneHooks, gpgProvider)...),
		WithComponents(components),
		WithDistros(config.Distros),
	)
	return apt, nil
}

// getAPTComponentsFromConfig converts the input map of component name, component file matchers to
// an output map of component name, component file matcher regexp instances. An error is returned if
// any of the provided component file matcher strings cannot be compiled into a regular expression.
func getAPTComponentsFromConfig(config map[string][]string) (map[string][]*regexp.Regexp, error) {
	components := make(map[string][]*regexp.Regexp, len(config))
	for componentName, fileMatchers := range config {
		fileMatcherExpressions := make([]*regexp.Regexp, 0, len(fileMatchers))
		for _, fileMatcher := range fileMatchers {
			fileMatcherExpression, err := regexp.Compile(fileMatcher)
			if err != nil {
				return nil, fmt.Errorf("failed to parse file matcher %q for APT component %q: %w", fileMatcher, componentName, err)
			}

			fileMatcherExpressions = append(fileMatcherExpressions, fileMatcherExpression)
		}

		components[componentName] = fileMatcherExpressions
	}

	return components, nil
}
