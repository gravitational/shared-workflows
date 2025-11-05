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
	"errors"
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/gpg"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
)

// This is a version of Publisher that always owns the complete lifecycle of dependent services, including cleanup.
// This differers from Publisher which expects service lifecycle to be handled by the caller.
type publisherFromConfig struct {
	*Publisher
	hooks []commandrunner.Hook
}

var _ ospackages.APTPublisher = (*publisherFromConfig)(nil)

// FromConfig creates a new Attune publisher instance from the provided config and Attune runner.
func FromConfig(ctx context.Context, config config.AttuneAPTPackagePublisher, logger *slog.Logger) (ospackages.APTPublisher, error) {
	authenticator, err := authenticators.FromConfig(config.Authentication)
	if err != nil {
		return nil, fmt.Errorf("failed to create Attune authenticator: %w", err)
	}

	hooks := []commandrunner.Hook{
		authenticator,
		gpg.FromConfig(config.GPG),
	}

	attuneRunner := commandrunner.NewRunner(commandrunner.WithLogger(logger), commandrunner.WithHooks(hooks...))
	publisher, err := NewPublisher(attuneRunner, WithLogger(logger))
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("failed to create new publisher: %w", err),
			// This must be called to prevent leakage of sensitive resources (e.g. GPG keys, certs)
			// This needs to use a separate context to ensure that cleanup is not cancelled in the event that
			// ctx is cancelled. TODO figure out a better way of plumbing this from the caller.
			cleanupHooks(context.TODO(), hooks),
		)
	}

	return &publisherFromConfig{
		Publisher: publisher,
		hooks:     hooks,
	}, nil
}

func (pfc *publisherFromConfig) Close(ctx context.Context) error {
	errs := make([]error, 0, 2)
	if err := pfc.Publisher.Close(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to clean up publisher: %w", err))
	}

	if err := cleanupHooks(ctx, pfc.hooks); err != nil {
		errs = append(errs, fmt.Errorf("failed to cleanup all hooks: %w", err))
	}

	return errors.Join(errs...)
}

func cleanupHooks(ctx context.Context, hooks []commandrunner.Hook) error {
	errs := make([]error, 0, len(hooks))
	for _, hook := range hooks {
		if err := hook.Close(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up hook %s: %w", hook.Name(), err))
		}
	}

	return errors.Join(errs...)
}
