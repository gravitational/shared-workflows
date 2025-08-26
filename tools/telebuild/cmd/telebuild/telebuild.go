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
	// Darwin is the command for building darwin artifacts.
	Darwin DarwinCmd `cmd:"" help:"Build Teleport darwin artifacts"`

	// CommonFlags contains flags that are common to all commands in the Telebuild CLI.
	CommonFlags
}

// CommonFlags is a kong struct that contains the top-level flags that are common to all commands in the Telebuild CLI.
// It is meant to be embedded in other commands to avoid duplication of common flags.
type CommonFlags struct {
	// BuildDir is the directory where build artifacts will be output.
	BuildDir string `name:"build-dir" short:"b" type:"path" env:"${envPrefix}BUILD_DIR" default:"build/.telebuild" help:"Directory used for intermediate build artifacts."`
	// OutputDir is the directory where the final output will be placed.
	OutputDir string `name:"output-dir" short:"o" type:"path" env:"${envPrefix}OUTPUT_DIR" default:"build/artifacts" help:"Directory to place the resulting release artifacts."`
	// DryRun indicates whether to perform a dry run without actual building.
	DryRun bool `name:"dry-run" short:"d" env:"${envPrefix}DRY_RUN" help:"Perform a dry run without actual building."`
}

const (
	envPrefix = "TELEBUILD_"
)

const (
	// Architectures supported by Telebuild.
	ArchAMD64     = "amd64"
	ArchARM64     = "arm64"
	ArchUniversal = "universal" // For macOS universal binaries
)

func main() {
	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("telebuild"),
		kong.Description("A CLI tool for building Teleport release artifacts."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
		kong.Vars{
			"envPrefix": envPrefix,
		},
		darwinKongVars,
	)

	// Run the CLI and handle any errors.
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}
