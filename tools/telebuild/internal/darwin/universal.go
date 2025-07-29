package darwin

import (
	"errors"
	"os/exec"
	"path/filepath"
)

// BuildUniversalTarball builds a tarball that contains multi-architecture binaries for macOS.
// The "universal" binaries can run on both Intel and Apple Silicon Macs.
// This is useful for distributing a single tarball that works across different Mac architectures.
//
// This uses the `lipo` tool to create a universal binary from multiple architecture-specific binaries.
// The best way to find more information on `lipo` is to refer to the official Apple documentation or run `man lipo` in the terminal.
func (b *Builder) BuildUniversalTarball() error {
	for _, bin := range binaries {
		// Combine arm64 & amd64 binaries into universal binaries.
		cmd := exec.Command("lipo", "-create", "-output",
			filepath.Join(b.builddirUniversal, bin),
			filepath.Join(b.builddirAmd64, bin),
			filepath.Join(b.builddirArm64, bin),
		)

		if b.dryRun {
			b.log.Info("Dry run: would execute command", "command", cmd.String())
			continue
		}
		return errors.New("not implemented yet")
	}

	return nil
}
