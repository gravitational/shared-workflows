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

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"golang.org/x/sync/errgroup"
)

const EnvVarPrefix = "OPRT2_"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) (err error) {
	c, err := loadConfig(args)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, err := config.GetLogger(c.Logger)
	if err != nil {
		return fmt.Errorf("failed to create logger")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	ctx = logging.ToCtx(ctx, logger)

	authenticator, err := config.GetAttuneAuthenticator(c.Attune.Authentication)
	if err != nil {
		return fmt.Errorf("failed to get Attune authenticator: %w", err)
	}

	// Ensure that the cleanup hook is always run. This is important to avoid leaking Attune credentials.
	if closer, ok := authenticator.(commandrunner.CleanupHook); ok {
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			logger.DebugContext(cleanupCtx, "cleaning up authenticator")

			closerErr := closer.Cleanup(cleanupCtx)
			if closerErr != nil {
				closerErr = fmt.Errorf("failed to close authenticator, credentials may be leaked: %w", closerErr)
			}
			err = errors.Join(err, closerErr)
		}()
	}

	packageManagers, closablePackageManagers, err := config.GetPackageManagers(ctx, c.PackageManagers, authenticator)
	// Cleanup must occur for all created package managers even if an error is returned. This ensures that if some are
	// created but some fail, the ones that were created don't leak.
	defer func() {
		logger.DebugContext(context.TODO(), "cleaning up package managers")

		errs := make([]error, 0, len(closablePackageManagers))
		for _, closablePackageManager := range closablePackageManagers {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			logger.DebugContext(cleanupCtx, fmt.Sprintf("cleaining up %s", closablePackageManager.Name()))

			errs = append(errs, closablePackageManager.Close(cleanupCtx))
			cancel()
		}

		err = errors.Join(append([]error{err}, errs...)...)
	}()

	if err != nil {
		return fmt.Errorf("failed to load all package managers: %w", err)
	}

	// Publish with all configured package managers
	publishingQueue, queueContext := errgroup.WithContext(ctx)
	publishingQueue.SetLimit(int(c.Attune.ParallelUploadLimit))

	for _, packageManager := range packageManagers {
		if err := packageManager.EnqueueForPublishing(queueContext, publishingQueue); err != nil {
			return fmt.Errorf("failed to publish with package manager %q: %w", packageManager.Name(), err)
		}
	}

	if err := publishingQueue.Wait(); err != nil {
		return fmt.Errorf("publishing failed: %w", err)
	}

	return nil
}

func loadConfig(args []string) (*config.OPRT2, error) {
	var configFilePath string

	kingpin.Flag("config-file", "Path to the config file.").
		Short('c').
		Envar(EnvVarPrefix + "CONFIG_FILE").
		Default("config.yaml").
		StringVar(&configFilePath)

	kingpin.MustParse(kingpin.CommandLine.Parse(args))

	config, err := config.ParseOPRT2ConfigFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file at %q: %w", configFilePath, err)
	}

	return config, nil
}
