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
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	filemanagerloader "github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager/loader"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages/publishers"
)

// This is a version of Manager that always owns the complete lifecycle of dependent services, including cleanup.
// This differers from Manager which expects service lifecycle to be handled by the caller.
type managerFromConfig struct {
	*Manager
	fileManager filemanager.FileManager
	publisher   ospackages.APTPublisher
}

var _ ospackages.Manager = (*managerFromConfig)(nil)

// FromConfig creates a new APT instance from the provided config and attune runner.
func FromConfig(ctx context.Context, config config.APTPackageManager, logger *slog.Logger) (ospackages.Manager, error) {
	repos, err := getAPTReposFromConfig(config.Repos)
	if err != nil {
		return nil, fmt.Errorf("failed to get APT repos: %w", err)
	}

	fileManager, err := filemanagerloader.FromConfig(ctx, config.FileSource)
	if err != nil {
		return nil, fmt.Errorf("failed to create file manager from config: %w", err)
	}

	publisher, err := publishers.FromAPTConfig(ctx, config.PublishingTool, logger)
	if err != nil {
		cleanupErr := fileManager.Close(context.TODO())
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("failed to close file manager %s: %w", fileManager.Name(), err)
		}
		return nil, errors.Join(fmt.Errorf("failed to create APT publisher: %w", err), cleanupErr)
	}

	return &managerFromConfig{
		Manager: NewManager(
			fileManager,
			WithLogger(logger),
			WithPublisher(publisher),
			WithRepos(repos),
		),
		fileManager: fileManager,
		publisher:   publisher,
	}, nil
}

// getAPTComponentsFromConfig converts the input map's values from strings to regexp instances.
func getAPTReposFromConfig(config map[string]map[string]map[string][]string) (map[string]map[string]map[string][]*regexp.Regexp, error) {
	convertedRepos := make(map[string]map[string]map[string][]*regexp.Regexp, len(config))

	for repoName, distributions := range config {
		convertedDistributions := make(map[string]map[string][]*regexp.Regexp, len(distributions))

		for distribution, components := range distributions {
			convertedComponents := make(map[string][]*regexp.Regexp, len(config))

			for componentName, fileMatchers := range components {
				fileMatcherExpressions := make([]*regexp.Regexp, 0, len(fileMatchers))
				for _, fileMatcher := range fileMatchers {
					fileMatcherExpression, err := regexp.Compile(fileMatcher)
					if err != nil {
						return nil, fmt.Errorf("failed to parse file matcher %q for APT component %q: %w", fileMatcher, componentName, err)
					}

					fileMatcherExpressions = append(fileMatcherExpressions, fileMatcherExpression)
				}

				convertedComponents[componentName] = fileMatcherExpressions
			}

			convertedDistributions[distribution] = convertedComponents
		}

		convertedRepos[repoName] = convertedDistributions
	}

	return convertedRepos, nil
}

func (afc *managerFromConfig) Close(ctx context.Context) error {
	errs := make([]error, 0, 2)
	if afc.Manager != nil {
		if err := afc.Manager.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up manager %s: %w", afc.Name(), err))
		}
	}

	if afc.fileManager != nil {
		if err := afc.fileManager.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up file manager %s: %w", afc.fileManager.Name(), err))
		}
	}

	if afc.publisher != nil {
		if err := afc.publisher.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up publisher %s: %w", afc.publisher.Name(), err))
		}
	}

	return errors.Join(errs...)
}
