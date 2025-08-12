package main

import (
	"errors"

	"github.com/gravitational/shared-workflows/tools/telebuild/notarize"
	"github.com/gravitational/shared-workflows/tools/telebuild/packaging/packageinstaller"
)

// MacOSCmd is a kong struct that contains the subcommands for macOS packaging and notarization.
type MacOSCmd struct {
	// Notarize contains the subcommand for notarizing files.
	Notarize NotarizeCmd `cmd:"" help:"Utility for notarizing files"`
	// AppBundle creates an Application Bundle (.app).
	AppBundle AppBundleCmd `cmd:"" help:"Create an Application Bundle (.app)"`
	// PackageInstaller creates a Package Installer (.pkg).
	PackageInstaller PackageInstallerCmd `cmd:"" help:"Create a package installer (.pkg)"`
}

// NotaryTool is a kong struct that contains flags for notarization options and credentials.
type NotaryTool struct {
	Retry  int  `group:"Notarization Optional Flags" help:"Retry notarization in case of failure."`
	DryRun bool `group:"Notarization Optional Flags" help:"Dry run notarization."`

	KeychainProfile string `group:"Notarization Required Flags" env:"KEYCHAIN_PROFILE" help:"Keychain profile to use for notarization. Use \"man notarytool\" for authentication options."`
	SigningID       string `group:"Notarization Required Flags" env:"SIGNING_ID" help:"Signing Identity to use for codesigning."`
	TeamID          string `group:"Notarization Required Flags" env:"TEAM_ID" help:"Team ID is the unique identifier for the Apple Developer account."`

	CI bool `hidden:"" env:"CI" help:"CI mode. Disables dry-run."`

	notaryTool *notarize.Tool
}

// NotarizeCmd is a kong struct that contains the command for notarizing files.
type NotarizeCmd struct {
	Files []string `arg:"" help:"List of files to notarize."`

	NotaryTool
}

type AppBundleCmd struct {
	AppBinary string `arg:"" help:"Binary to use as the main executable for the app bundle."`
	Skeleton  string `arg:"" help:"Skeleton directory to use as the base for the app bundle."`

	Entitlements string `flag:"" help:"Entitlements file to use for the app bundle."`
	BundleID     string `flag:"" required:"" help:"Bundle ID is a unique identifier for the app bundle. Required for notarization."`

	NotaryTool
}

type PackageInstallerCmd struct {
	RootPath          string `arg:"" help:"Path to the root directory of the package installer."`
	PackageOutputPath string `arg:"" help:"Path to the output package installer."`

	InstallLocation string `flag:"" required:"" help:"Location where the package contents will be installed."`
	BundleID        string `flag:"" required:"" help:"Bundle ID is a unique identifier for the package installer."`
	ScriptsDir      string `flag:"" help:"Path to the scripts directory. Contains preinstall and postinstall scripts."`
	Version         string `flag:"" help:"Version of the package. Used in determining upgrade behavior."`

	NotaryTool
}

func (cmd *PackageInstallerCmd) Run(cli *CLI) error {
	pkg, err := packageinstaller.NewPackager(
		packageinstaller.Info{
			RootPath:        cmd.RootPath,
			InstallLocation: cmd.InstallLocation,
			OutputPath:      cmd.PackageOutputPath,
			ScriptsDir:      cmd.ScriptsDir,
			BundleID:        cmd.BundleID,
		},
		packageinstaller.WithLogger(log),
		packageinstaller.WithNotaryTool(cmd.notaryTool),
	)
	if err != nil {
		return err
	}

	return pkg.Package()
}

func (cmd *NotarizeCmd) Run(cli *CLI) error {
	return cmd.notaryTool.NotarizeBinaries(cmd.Files)
}

func (n *NotaryTool) AfterApply() error {
	// Dry run if no credentials are provided
	credsMissing := n.KeychainProfile == "" || n.SigningID == "" || n.TeamID == ""

	if !n.DryRun && credsMissing {
		return errors.New("notarization credentials required, use --dry-run to skip")
	}

	if n.CI && n.DryRun {
		return errors.New("--dry-run mode cannot be used in CI")
	}

	extraOpts := []notarize.Opt{}

	if n.DryRun {
		extraOpts = append(extraOpts, notarize.DryRun())
	}

	t, err := notarize.NewTool(
		notarize.Creds{
			KeychainProfile: n.KeychainProfile,
			SigningIdentity: n.SigningID,
			TeamID:          n.TeamID,
		},
		append(
			extraOpts,
			notarize.MaxRetries(n.Retry),
			notarize.WithLogger(log),
		)...,
	)

	if err != nil {
		return err
	}

	n.notaryTool = t

	return nil
}
