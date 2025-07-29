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

import "github.com/alecthomas/kong"

// CLI is the root kong struct for the Telebuild CLI for building Teleport release artifacts.
// It contains top-level flags and subcommands for building tarballs, packages, and other artifacts.
type CLI struct {
	// Tarball is the command for building a tarball containing binaries for Teleport.
	Tarball TarballCmd `cmd:"" help:"Build a tarball containing binaries for Teleport"`

	// CommonFlags contains flags that are common to all commands in the Telebuild CLI.
	CommonFlags
}

// CommonFlags is a kong struct that contains the top-level flags that are common to all commands in the Telebuild CLI.
// It is meant to be embedded in other commands to avoid duplication of common flags.
type CommonFlags struct {
	// BuildDir is the directory where build artifacts will be output.
	BuildDir string `name:"build-dir" short:"b" type:"path" env:"TELEBUILD_BUILD_DIR" default:"build/.telebuild" help:"Directory used for intermediate build artifacts."`
	// OutputDir is the directory where the final output will be placed.
	OutputDir string `name:"output-dir" short:"o" type:"path" env:"TELEBUILD_OUTPUT_DIR" default:"build/artifacts" help:"Directory to place the resulting release artifacts."`
	// DryRun indicates whether to perform a dry run without actual building.
	DryRun bool `name:"dry-run" short:"d" env:"TELEBUILD_DRY_RUN" help:"Perform a dry run without actual building."`
}

const (
	// Operating Systems supported by Telebuild.
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"
)

const (
	// Architectures supported by Telebuild.
	ArchAMD64     = "amd64"
	ArchARM64     = "arm64"
	ArchUniversal = "universal" // For macOS universal binaries
)

// PlatformFlags is a kong struct that contains common flags for specifying platform specific configuration.
// This is meant to be embedded in other commands that require platform-specific flags.
type PlatformFlags struct {
	// OS is the target operating system for the build.
	OS string `help:"Target operating system" enum:"linux,darwin,windows" required:""`
	// Arch is the target architecture for the build.
	Arch string `help:"Target architecture" enum:"amd64,arm64,universal" required:""`
}

func main() {
	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("telebuild"),
		kong.Description("A CLI tool for building Teleport release artifacts."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	// Run the CLI and handle any errors.
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}
