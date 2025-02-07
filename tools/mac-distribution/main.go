package main

import (
	"fmt"
	"log/slog"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging"
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
	Retry             int  `group:"notarization options" help:"Retry notarization in case of failure."`
	ForceNotarization bool `group:"notarization options" help:"Always attempt notarization. By default notarization will be skipped if apple-password or apple-username is not set."`

	AppleUsername string `group:"notarization creds" and:"notarization creds" env:"APPLE_USERNAME" help:"Apple Username. Required for notarization. Must use with apple-password."`
	ApplePassword string `group:"notarization creds" and:"notarization creds" env:"APPLE_PASSWORD" help:"Apple Password. Required for notarization. Must use with apple-username."`
	SigningID     string `group:"notarization creds" and:"notarization creds" env:"SIGNING_ID" help:"Signing Identity to use for codesigning. Required for notarization."`
	BundleID      string `group:"notarization creds" and:"notarization creds" env:"BUNDLE_ID" help:"Bundle ID is a unique identifier used for codesigning & notarization. Required for notarization."`
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
	cli := CLI{
		GlobalFlags: GlobalFlags{},
	}

	kctx := kong.Parse(&cli)
	err := kctx.Run(&cli.GlobalFlags)
	kctx.FatalIfErrorf(err)
}

func (c *AppBundleCmd) Run(g *GlobalFlags) error {
	notaryTool, err := g.InitNotaryTool()
	if err != nil {
		return trace.Wrap(err)
	}

	pkg := packaging.NewAppBundlePackager(
		&packaging.AppBundleInfo{
			Skeleton:     c.Skeleton,
			Entitlements: c.Entitlements,
			AppBinary:    c.AppBinary,
		},
		&packaging.AppBundlePackagerOpts{
			NotaryTool: notaryTool,
			Logger:     log,
		},
	)

	return pkg.Package()
}

func (c *PackageInstallerCmd) Run(g *GlobalFlags) error {
	notaryTool, err := g.InitNotaryTool()
	if err != nil {
		return trace.Wrap(err)
	}

	pkg := packaging.NewPackageInstallerPackager(
		&packaging.PackageInstallerInfo{
			RootPath:        c.RootPath,
			InstallLocation: c.InstallLocation,
			OutputPath:      c.PackageOutputPath,
			ScriptsDir:      c.ScriptsDir,
			BundleID:        g.BundleID, // Only populated for notarization
		},
		&packaging.PackageInstallerPackagerOpts{
			NotaryTool: notaryTool,
			Logger:     log,
		},
	)

	return pkg.Package()
}

func (c *NotarizeCmd) Run(g *GlobalFlags) error {
	tool, err := g.InitNotaryTool()
	if err != nil {
		return trace.Wrap(err)
	}
	return trace.Wrap(tool.NotarizeBinaries(c.Files))
}

func (g *GlobalFlags) InitNotaryTool() (*notarize.Tool, error) {
	// Dry run if no credentials are provided
	dryRun := g.AppleUsername == "" || g.ApplePassword == "" || g.SigningID == ""
	if dryRun && g.ForceNotarization {
		return nil, fmt.Errorf("notarization credentials not provided")
	}

	if dryRun {
		log.Warn("notarization dry run enabled", "reason", "notarization credentials missing")
	}

	// Initialize notary tool
	return notarize.NewTool(
		notarize.Creds{
			AppleUsername:   g.AppleUsername,
			ApplePassword:   g.ApplePassword,
			SigningIdentity: g.SigningID,
			BundleID:        g.BundleID,
		},
		notarize.ToolOpts{
			Retry:  g.Retry,
			DryRun: dryRun,
			Logger: log,
		},
	), nil
}
