package main

import (
	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/packaging"
)

type Task interface {
	Run()
}

type CLI struct {
	// Subcommands
	Notarize         NotarizeCmd         `cmd:"" help:"Utility for notarizing files"`
	Wait             WaitCmd             `cmd:"" help:"Wait for a notarization submission to complete"`
	AppBundle        AppBundleCmd        `cmd:"" group:"packaging" help:"Create an Application Bundle (.app)"`
	PackageInstaller PackageInstallerCmd `cmd:"" group:"packaging" help:"Create a package installer (.pkg)"`

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
	BundleID        string `flag:"" required:"" help:"Unique identifier for the package installer."`
	ScriptsDir      string `flag:"" help:"Path to the scripts directory. Contains preinstall and postinstall scripts."`
	Version         string `flag:"" help:"Version of the package. Used in determining upgrade behavior."`
}

type WaitCmd struct{}

func main() {
	cli := CLI{
		GlobalFlags: GlobalFlags{},
	}

	kctx := kong.Parse(&cli)
	err := kctx.Run(&cli.GlobalFlags)
	kctx.FatalIfErrorf(err)
}

func (c *AppBundleCmd) Run(g *GlobalFlags) error {
	pkg := packaging.NewAppBundlePackager(
		&packaging.AppBundleInfo{
			Skeleton:     c.Skeleton,
			Entitlements: c.Entitlements,
			AppBinary:    c.AppBinary,
		},
		&packaging.AppBundlePackagerOpts{},
	)

	return pkg.Package()
}

func (c *PackageInstallerCmd) Run(g *GlobalFlags) error {
	pkg := packaging.NewPackageInstallerPackager(
		&packaging.PackageInstallerInfo{
			RootPath:        c.RootPath,
			InstallLocation: c.InstallLocation,
			BundleID:        c.BundleID,
			PackageName:     c.PackageOutputPath,
			ScriptsDir:      c.ScriptsDir,
		},
		&packaging.PackageInstallerPackagerOpts{},
	)

	return pkg.Package()
}
