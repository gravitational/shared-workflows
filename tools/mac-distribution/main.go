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
	"errors"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging/appbundle"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging/packageinstaller"
)

var log = slog.Default()

type CLI struct {
	// Subcommands
	Notarize   NotarizeCmd         `cmd:"" help:"Utility for notarizing files"`
	PackageApp AppBundleCmd        `cmd:"" help:"Create an Application Bundle (.app)"`
	PackagePkg PackageInstallerCmd `cmd:"" help:"Create a package installer (.pkg)"`
}

type NotarizeCmd struct {
	Files []string `arg:"" help:"List of files to notarize."`

	NotaryCmd
}

type NotaryCmd struct {
	Retry  int  `group:"notarization options" help:"Retry notarization in case of failure."`
	DryRun bool `group:"notarization options" help:"Dry run notarization."`

	AppleUsername string `group:"notarization creds" env:"APPLE_USERNAME" help:"Apple Username. Required for notarization. Must use with apple-password."`
	ApplePassword string `group:"notarization creds" env:"APPLE_PASSWORD" help:"Apple Password. Required for notarization. Must use with apple-username."`
	SigningID     string `group:"notarization creds" env:"SIGNING_ID" help:"Signing Identity to use for codesigning. Required for notarization."`
	TeamID        string `group:"notarization creds" env:"TEAM_ID" help:"Team ID is the unique identifier for the Apple Developer account."`

	CI bool `hidden:"" env:"CI" help:"CI mode. Disables dry-run."`

	notaryTool *notarize.Tool
}

type AppBundleCmd struct {
	AppBinary string `arg:"" help:"Binary to use as the main executable for the app bundle."`
	Skeleton  string `arg:"" help:"Skeleton directory to use as the base for the app bundle."`

	Entitlements string `flag:"" help:"Entitlements file to use for the app bundle."`
	BundleID     string `flag:"" required:"" help:"Bundle ID is a unique identifier for the app bundle. Required for notarization."`

	NotaryCmd
}

type PackageInstallerCmd struct {
	RootPath          string `arg:"" help:"Path to the root directory of the package installer."`
	PackageOutputPath string `arg:"" help:"Path to the output package installer."`

	InstallLocation string `flag:"" required:"" help:"Location where the package contents will be installed."`
	BundleID        string `flag:"" required:"" help:"Bundle ID is a unique identifier for the package installer."`
	ScriptsDir      string `flag:"" help:"Path to the scripts directory. Contains preinstall and postinstall scripts."`
	Version         string `flag:"" help:"Version of the package. Used in determining upgrade behavior."`

	NotaryCmd
}

func main() {
	var cli CLI

	kctx := kong.Parse(&cli)
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

func (c *PackageInstallerCmd) Run(cli *CLI) error {
	pkg, err := packageinstaller.NewPackager(
		packageinstaller.Info{
			RootPath:        c.RootPath,
			InstallLocation: c.InstallLocation,
			OutputPath:      c.PackageOutputPath,
			ScriptsDir:      c.ScriptsDir,
			BundleID:        c.BundleID,
		},
		packageinstaller.WithLogger(log),
		packageinstaller.WithNotaryTool(c.notaryTool),
	)
	if err != nil {
		return err
	}

	return pkg.Package()
}

func (c *PackageInstallerCmd) Validate() error {
	if c.BundleID == "" {
		return errors.New("Bundle ID is required for package installer regardless of notarization")
	}

	return nil
}

func (c *NotarizeCmd) Run(cli *CLI) error {
	return c.notaryTool.NotarizeBinaries(c.Files)
}

func (g *NotaryCmd) AfterApply() error {
	// Dry run if no credentials are provided
	credsMissing := g.AppleUsername == "" || g.ApplePassword == "" || g.SigningID == "" || g.TeamID == ""

	if !g.DryRun && credsMissing {
		return errors.New("notarization credentials required, use --dry-run to skip")
	}

	if g.CI && g.DryRun {
		return errors.New("dry-run mode cannot be used in CI")
	}

	extraOpts := []notarize.Opt{}

	if g.DryRun {
		extraOpts = append(extraOpts, notarize.DryRun())
	}

	t, err := notarize.NewTool(
		notarize.Creds{
			AppleUsername:   g.AppleUsername,
			ApplePassword:   g.ApplePassword,
			SigningIdentity: g.SigningID,
			TeamID:          g.TeamID,
		},
		append(
			extraOpts,
			notarize.MaxRetries(g.Retry),
			notarize.WithLogger(log),
		)...,
	)

	if err != nil {
		return err
	}

	g.notaryTool = t

	return nil
}
