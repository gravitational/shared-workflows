package main

import (
	"fmt"

	"github.com/gravitational/shared-workflows/tools/telebuild/internal/darwin"
)

// TarballCmd is a kong struct that contains flags and methods for building a tarball.
// It is meant to be used as a subcommand of the Telebuild CLI.
// This command will handle building tarballs for different platforms, including a universal tarball for macOS.
type TarballCmd struct {
	// PlatformFlags contains flags that are specific to the platform being built.
	PlatformFlags
}

// Run executes the tarball build process.
func (cmd *TarballCmd) Run(cli *CLI) error {
	switch {
	case cmd.OS == OSDarwin && cmd.Arch == ArchUniversal:
		return cmd.buildDarwinUniversalTarball(cli)
	default:
		return fmt.Errorf("unsupported OS/Arch combination: %s/%s", cmd.OS, cmd.Arch)
	}
}

func (cmd *TarballCmd) buildDarwinUniversalTarball(cli *CLI) error {
	// Placeholder to show the use of common top-level flags
	if cli.DryRun {
		fmt.Println("Dry run: would build a universal tarball for darwin")
		return nil
	}

	builder, err := darwin.NewUniversalTarballBuilder()
	if err != nil {
		return fmt.Errorf("initializing darwin universal tarball builder: %w", err)
	}

	if err := builder.Build(); err != nil {
		return fmt.Errorf("building darwin universal tarball: %w", err)
	}

	return nil
}
