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
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/telebuild/internal/darwin"
)

// TarballCmd is a kong struct that contains flags and methods for building a tarball.
// It is meant to be used as a subcommand of the Telebuild CLI.
// This command will handle building tarballs for different platforms, including a universal tarball for macOS.
type TarballCmd struct {
	// PlatformFlags contains flags that are specific to the platform being built.
	PlatformFlags
}

// Run executes the tarball build process.
func (cmd *TarballCmd) Run(cli *CLI) error {
	switch {
	case cmd.OS == OSDarwin && cmd.Arch == ArchUniversal:
		return cmd.buildDarwinUniversalTarball(cli)
	default:
		return fmt.Errorf("unsupported OS/Arch combination: %s/%s", cmd.OS, cmd.Arch)
	}
}

func (cmd *TarballCmd) buildDarwinUniversalTarball(cli *CLI) error {
	opts := []darwin.BuilderOpt{
		darwin.WithLogger(slog.Default().With("task", "darwin-universal-tarball")),
		darwin.WithBuildDir(cli.BuildDir),
		darwin.WithOutputDir(cli.OutputDir),
	}

	if cli.DryRun {
		opts = append(opts, darwin.WithDryRun(cli.DryRun))
	}

	builder, err := darwin.NewBuilder(opts...)
	if err != nil {
		return fmt.Errorf("failed to create darwin builder: %w", err)
	}

	if err := builder.BuildUniversalTarball(); err != nil {
		return fmt.Errorf("failed to build universal tarball: %w", err)
	}

	return nil
}
