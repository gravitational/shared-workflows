package packageinstaller

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/exec"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/trace"
)

// Packager creates a package installer (.pkg) for distribution.
type Packager struct {
	Info Info

	log        *slog.Logger
	notaryTool *notarize.Tool
	cmdRunner  exec.CommandRunner
	dryRun     bool
}

// Info represents a package installer to be packaged for distribution.
type Info struct {
	// RootPath should contain the entire contents you want to package.
	RootPath string
	// InstallLocation is the location where the package contents will be installed.
	InstallLocation string
	// BundleID is a unique identifier for the package installer.
	// This is typically in reverse domain notation.
	// 		Example: com.gravitational.teleport.myapp
	BundleID string
	// OutputPath is desired output path of the package installer.
	OutputPath string

	// Optional fields
	// ScriptsDir is the path to the scripts directory.
	ScriptsDir string
	// Version is the version of the package.
	Version string
}

// Opt is a functional option for configuring a Packager.
type Opt func(*Packager)

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
}

// NewPackager creates a new PackageInstallerPackager.
func NewPackager(info Info, opts ...Opt) (*Packager, error) {
	if err := info.validate(); err != nil {
		return nil, trace.Wrap(err)
	}
	pkg := &Packager{
		Info: info,
	}
	for _, opt := range defaultOpts {
		opt(pkg)
	}

	for _, opt := range opts {
		opt(pkg)
	}

	var runner exec.CommandRunner = &exec.DefaultCommandRunner{}
	if pkg.dryRun {
		runner = exec.NewDryRunner(pkg.log)
	}
	pkg.cmdRunner = runner

	return pkg, nil
}

// Package creates a package installer.
func (p *Packager) Package() error {
	// Create a plist file for the package installer
	tmpdir, err := os.MkdirTemp("", "packageinstaller-*")
	if err != nil {
		return trace.Wrap(err, "failed to create temp dir")
	}
	defer os.RemoveAll(tmpdir)
	plistPath := filepath.Join(tmpdir, filepath.Base(p.Info.OutputPath)+".plist")
	if err := p.nonRelocatablePlist(plistPath); err != nil {
		return trace.Wrap(err)
	}

	args := []string{
		"--root", p.Info.RootPath,
		"--install-location", p.Info.InstallLocation,
		"--component-plist", plistPath,
	}

	if p.Info.BundleID != "" {
		args = append(args, "--identifier", p.Info.BundleID)
	}

	if p.Info.ScriptsDir != "" {
		args = append(args, "--scripts", p.Info.ScriptsDir)
	}

	if p.Info.Version != "" {
		args = append(args, "--version", p.Info.Version)
	}

	p.log.Info("building package installer...")
	args = append(args, p.Info.OutputPath)
	_, err = p.cmdRunner.RunCommand("pkgbuild", args...)
	if err != nil {
		return trace.Wrap(err, "failed to create package installer")
	}

	if p.notaryTool != nil {
		if err := p.notaryTool.NotarizePackageInstaller(p.Info.OutputPath, p.Info.OutputPath); err != nil {
			return trace.Wrap(err)
		}
	}

	p.log.Info("successfully created package installer", "path", p.Info.OutputPath)
	return nil
}

// nonRelocatablePlist analyzes the root path to create a component plist file.
// This plist is then modified to be non-relocatable.
func (p *Packager) nonRelocatablePlist(plistPath string) error {
	p.log.Info("analyzing package...")
	_, err := p.cmdRunner.RunCommand("pkgbuild", "--analyze", "--root", p.Info.RootPath, plistPath)
	if err != nil {
		return trace.Wrap(err, "failed to analyze package installer")
	}

	// todo: Use a plist library instead of shelling out to replace plist attributes.
	//       It's currently convenient to shell out to confirm parity to existing pipeline code

	// Set BundleIsRelocatable to false for consistency.
	// We have a lot of automation scripts that expect binaries to be in a specific location.
	_, err = p.cmdRunner.RunCommand("plutil", "-replace", "BundleIsRelocatable", "-bool", "NO", plistPath)
	if err != nil {
		return trace.Wrap(err, "failed to modify plist")
	}

	// Set BundleIsVersionChecked to false to allow for downgrades of the package.
	// Normal operation is to only allow version upgrades to overwrite. This disables that.
	_, err = p.cmdRunner.RunCommand("plutil", "-replace", "BundleIsVersionChecked", "-bool", "NO", plistPath)
	if err != nil {
		return trace.Wrap(err, "failed to modify plist")
	}

	p.log.Info("created component plist", "path", plistPath)
	return nil
}

func (p *Info) validate() error {
	if p.RootPath == "" {
		return trace.BadParameter("root path is required")
	}

	if p.OutputPath == "" {
		return trace.BadParameter("output path is required")
	}

	if p.BundleID == "" {
		return trace.BadParameter("bundle ID is required")
	}

	if p.InstallLocation == "" {
		return trace.BadParameter("install location is required")
	}

	return nil
}

// WithLogger sets the logger for the packager.
// By default, the packager will use slog.Default().
func WithLogger(log *slog.Logger) Opt {
	return func(a *Packager) {
		a.log = log
	}
}

// WithNotaryTool sets the notary tool for the packager.
func WithNotaryTool(tool *notarize.Tool) Opt {
	return func(a *Packager) {
		a.notaryTool = tool
	}
}

// DryRun sets the packager to dry run mode.
// In dry run mode, the packager will not execute commands and will not actually create the package installer.
func DryRun() Opt {
	return func(a *Packager) {
		a.dryRun = true
	}
}
