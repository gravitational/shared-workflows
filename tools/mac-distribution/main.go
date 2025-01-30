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

type PackageInstallerCmd struct{}

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
	pkg := packaging.AppBundle{
		Skeleton:     c.Skeleton,
		Entitlements: c.Entitlements,
		AppBinary:    c.AppBinary,
	}

	return pkg.Build()
}
