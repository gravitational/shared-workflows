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
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/telebuild/packaging/appbundle"
)

var log = slog.Default()

// CLI is a kong struct that defines the command line interface for the telebuild tool.
type CLI struct {
	// MacOS contains the subcommands for macOS packaging and notarization.
	MacOS MacOSCmd `cmd:"" name:"macos" help:"macOS packaging and notarization tools"`
}

func main() {
	var cli CLI

	kctx := kong.Parse(&cli,
		kong.Name("telebuild"),
		kong.Description("A tool for building and notarizing macOS applications and packages."),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}

func (c *AppBundleCmd) Run(cli *CLI) error {
	pkg, err := appbundle.NewPackager(
		appbundle.Info{
			Skeleton:     c.Skeleton,
			Entitlements: c.Entitlements,
			AppBinary:    c.AppBinary,
		},
		appbundle.WithLogger(log),
		appbundle.WithNotaryTool(c.notaryTool),
		appbundle.WithBundleID(c.BundleID),
	)
	if err != nil {
		return err
	}

	return pkg.Package()
}
