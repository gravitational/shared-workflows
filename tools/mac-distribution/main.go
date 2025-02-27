package main

import (
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging/appbundle"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging/packageinstaller"
	"github.com/gravitational/trace"
)

var log = slog.Default()

type CLI struct {
	// Subcommands
	Notarize   NotarizeCmd         `cmd:"" help:"Utility for notarizing files"`
	PackageApp AppBundleCmd        `cmd:"" help:"Create an Application Bundle (.app)"`
	PackagePkg PackageInstallerCmd `cmd:"" help:"Create a package installer (.pkg)"`

	GlobalFlags
}

type NotarizeCmd struct {
	Files []string `arg:"" help:"List of files to notarize."`
}

type GlobalFlags struct {
	Retry  int  `group:"notarization options" help:"Retry notarization in case of failure."`
	DryRun bool `group:"notarization options" help:"Dry run notarization."`

	AppleUsername string `group:"notarization creds" and:"notarization creds" env:"APPLE_USERNAME" help:"Apple Username. Required for notarization. Must use with apple-password."`
	ApplePassword string `group:"notarization creds" and:"notarization creds" env:"APPLE_PASSWORD" help:"Apple Password. Required for notarization. Must use with apple-username."`
	SigningID     string `group:"notarization creds" and:"notarization creds" env:"SIGNING_ID" help:"Signing Identity to use for codesigning. Required for notarization."`
	BundleID      string `group:"notarization creds" env:"BUNDLE_ID" help:"Bundle ID is a unique identifier used for codesigning & notarization. Required for notarization."`
	TeamID        string `group:"notarization creds" and:"notarization creds" env:"TEAM_ID" help:"Team ID is the unique identifier for the Apple Developer account."`

	CI bool `hidden:"" env:"CI" help:"CI mode. Disables dry-run."`

	notaryTool *notarize.Tool
}

type AppBundleCmd struct {
	AppBinary string `arg:"" help:"Binary to use as the main executable for the app bundle."`
	Skeleton  string `arg:"" help:"Skeleton directory to use as the base for the app bundle."`

	Entitlements string `flag:"" help:"Entitlements file to use for the app bundle."`
}

type PackageInstallerCmd struct {
	RootPath          string `arg:"" help:"Path to the root directory of the package installer."`
	PackageOutputPath string `arg:"" help:"Path to the output package installer."`

	InstallLocation string `flag:"" required:"" help:"Location where the package contents will be installed."`
	ScriptsDir      string `flag:"" help:"Path to the scripts directory. Contains preinstall and postinstall scripts."`
	Version         string `flag:"" help:"Version of the package. Used in determining upgrade behavior."`
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
		appbundle.WithNotaryTool(cli.notaryTool),
	)
	if err != nil {
		return trace.Wrap(err)
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
			BundleID:        cli.BundleID, // Only populated for notarization
		},
		packageinstaller.WithLogger(log),
		packageinstaller.WithNotaryTool(cli.notaryTool),
	)
	if err != nil {
		return trace.Wrap(err)
	}

	return pkg.Package()
}

func (c *NotarizeCmd) Run(cli *CLI) error {
	return trace.Wrap(cli.notaryTool.NotarizeBinaries(c.Files))
}

func (g *GlobalFlags) AfterApply() error {
	// Dry run if no credentials are provided
	credsMissing := g.AppleUsername == "" || g.ApplePassword == "" || g.SigningID == "" || g.BundleID == "" || g.TeamID == ""

	if !g.DryRun && credsMissing {
		return trace.BadParameter("notarization credentials required, use --dry-run to skip")
	}

	if g.CI && g.DryRun {
		return trace.BadParameter("dry-run mode cannot be used in CI")
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
			BundleID:        g.BundleID,
			TeamID:          g.TeamID,
		},
		append(
			extraOpts,
			notarize.MaxRetries(g.Retry),
			notarize.WithLogger(log),
		)...,
	)

	if err != nil {
		return trace.Wrap(err)
	}

	g.notaryTool = t

	return nil
}
