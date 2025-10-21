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
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/packagemanager"
	"golang.org/x/sync/errgroup"
)

const EnvVarPrefix = "OPRT2_"

func main() {
	var configFilePath string

	kingpin.Flag("config-file", "Path to the config file.").
		Short('c').
		Envar(EnvVarPrefix + "CONFIG_FILE").
		Default("config.yaml").
		StringVar(&configFilePath)

	kingpin.MustParse(kingpin.CommandLine.Parse(os.Args[1:]))

	if err := run(configFilePath); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(configFilePath string) (err error) {
	c, err := loadConfig(configFilePath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, err := logging.NewLogger(*c.Logger)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	authenticator, err := authenticators.FromConfig(c.Attune.Authentication)
	if err != nil {
		return fmt.Errorf("failed to get Attune authenticator: %w", err)
	}

	// Ensure that the cleanup hook is always run. This is important to avoid leaking Attune credentials.
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		logger.DebugContext(cleanupCtx, "cleaning up authenticator")

		closerErr := authenticator.Cleanup(cleanupCtx)
		if closerErr != nil {
			closerErr = fmt.Errorf("failed to close authenticator, credentials may be leaked: %w", closerErr)
		}
		err = errors.Join(err, closerErr)
	}()

	// Build a list of package managers based on the config
	packageManagers := make([]packagemanager.Manager, 0, len(c.PackageManagers))
	for _, packageManagerConfig := range c.PackageManagers {
		packageManager, err := packagemanager.FromConfig(ctx, packageManagerConfig, authenticator)
		if err != nil {
			return fmt.Errorf("failed to create package manager from config: %w", err)
		}

		packageManagers = append(packageManagers, packageManager)
	}

	// Cleanup must occur for all created package managers even if an error is returned. This ensures that if some are
	// created but some fail, the ones that were created don't leak.
	defer func() {
		logger.DebugContext(context.TODO(), "cleaning up package managers")

		errs := make([]error, 0, len(packageManagers))
		for _, closablePackageManager := range packageManagers {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			logger.DebugContext(cleanupCtx, "cleaning up package manager", "name", closablePackageManager.Name())

			errs = append(errs, closablePackageManager.Close(cleanupCtx))
			cancel()
		}

		err = errors.Join(append([]error{err}, errs...)...)
	}()

	if err != nil {
		return fmt.Errorf("failed to load all package managers: %w", err)
	}

	// Collect package publishing tasks for all configured package managers
	packagePublishingTasks := make([]packagemanager.PackagePublishingTask, 0)
	for _, packageManager := range packageManagers {
		tasks, err := packageManager.GetPackagePublishingTasks(ctx)
		if err != nil {
			return fmt.Errorf("failed to collect publishing tasks for package manager %q: %w", packageManager.Name(), err)
		}
		packagePublishingTasks = append(packagePublishingTasks, tasks...)
	}

	// Run all publishing tasks
	publishingQueue, queueContext := errgroup.WithContext(ctx)
	if c.Attune.ParallelUploadLimit > 0 {
		publishingQueue.SetLimit(int(c.Attune.ParallelUploadLimit))
	}

	for _, task := range packagePublishingTasks {
		publishingQueue.Go(func() error {
			return task(queueContext)
		})
	}

	if err := publishingQueue.Wait(); err != nil {
		return fmt.Errorf("publishing failed: %w", err)
	}

	return nil
}

func loadConfig(configFilePath string) (*config.OPRT2, error) {
	config, err := config.ParseOPRT2ConfigFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file at %q: %w", configFilePath, err)
	}

	return config, nil
}
