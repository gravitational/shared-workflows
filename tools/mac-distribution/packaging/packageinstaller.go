package packaging

import (
	"log/slog"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/exec"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/trace"
)

// PackageInstallerPackager is a packager for creating a package installer (.pkg) for distribution.
type PackageInstallerPackager struct {
	Info *PackageInstallerInfo

	log        *slog.Logger
	notaryTool *notarize.Tool
	cmdRunner  exec.CommandRunner
}

// PackageInstallerInfo represents a package installer to be packaged for distribution.
type PackageInstallerInfo struct {
	// RootPath should contain the entire contents you want to package.
	RootPath string
	// InstallLocation is the location where the package contents will be installed.
	InstallLocation string
	// BundleID is a unique identifier for the package installer.
	// This is typically in reverse domain notation.
	// 		Example: com.gravitational.teleport.myapp
	BundleID string
	// PackageName is the name of the package installer
	// This is only used for the filename of the package installer.
	// This has no effect on the package itself.
	PackageName string

	// Optional fields
	// ScriptsDir is the path to the scripts directory.
	ScriptsDir string
	// Version is the version of the package.
	Version string
}

// PackageInstallerPackagerOpts contains options for creating a PackageInstallerPackager.
type PackageInstallerPackagerOpts struct {
	NotaryTool *notarize.Tool
	Logger     *slog.Logger
	CmdRunner  exec.CommandRunner
}

// NewPackageInstallerPackager creates a new PackageInstallerPackager.
func NewPackageInstallerPackager(info *PackageInstallerInfo, opts *PackageInstallerPackagerOpts) *PackageInstallerPackager {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	return &PackageInstallerPackager{
		Info:       info,
		log:        log,
		notaryTool: opts.NotaryTool,
		cmdRunner:  opts.CmdRunner,
	}
}

func (p *PackageInstallerPackager) Package() error {
	if err := p.Info.validate(); err != nil {
		return trace.Wrap(err)
	}

	// Create a plist file for the package installer
	plistPath := filepath.Join(p.Info.RootPath, ".plist")
	if err := p.nonRelocatablePlist(plistPath); err != nil {
		return trace.Wrap(err)
	}

	args := []string{
		"--root", p.Info.RootPath,
		"--identifier", p.Info.PackageName,
		"--version", p.Info.Version,
		"--install-location", p.Info.InstallLocation,
		"--component-plist", plistPath,
		p.Info.PackageName,
	}

	if p.Info.ScriptsDir != "" {
		args = append(args, "--scripts", p.Info.ScriptsDir)
	}

	if p.Info.Version != "" {
		args = append(args, "--version", p.Info.Version)
	}

	out, err := p.cmdRunner.RunCommand("pkgbuild", args...)
	if err != nil {
		return trace.Wrap(err, "failed to create package installer")
	}
	p.log.Info("pkgbuild output", "output", out)

	if p.notaryTool != nil {
		if err := p.notaryTool.NotarizePackageInstaller(p.Info.PackageName, p.Info.PackageName); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// nonRelocatablePlist analyzes the root path to create a compponent plist file.
// This plist is then modified to be non-relocatable.
func (p *PackageInstallerPackager) nonRelocatablePlist(plistPath string) error {
	out, err := p.cmdRunner.RunCommand("pkgbuild", "--analyze", p.Info.RootPath, plistPath)
	if err != nil {
		return trace.Wrap(err, "failed to analyze package installer")
	}
	p.log.Info("pkgbuild analyze output", "output", out)

	out, err = p.cmdRunner.RunCommand("plutil", "-replace", "BundleIsRelocatable", "-bool", "NO", plistPath)
	if err != nil {
		return trace.Wrap(err, "failed to modify plist")
	}
	p.log.Info("plutil output", "output", out)
	return nil
}

func (p *PackageInstallerInfo) validate() error {
	if p.RootPath == "" {
		return trace.BadParameter("root path is required")
	}

	if p.PackageName == "" {
		return trace.BadParameter("package name is required")
	}

	if p.BundleID == "" {
		return trace.BadParameter("bundle ID is required")
	}

	if p.InstallLocation == "" {
		return trace.BadParameter("install location is required")
	}

	return nil
}
