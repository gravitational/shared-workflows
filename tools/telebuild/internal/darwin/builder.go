package darwin

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
)

// list of binaries built for darwin systems
var binaries = []string{
	"fdpass-teleport",
	"tbot",
	"tctl",
	"teleport",
	"tsh",
}

// Builder handles the creation of Darwin-specific release artifacts such as tarballs, package installers, etc.
// It manages the build process for macOS targets, supporting both Intel (amd64) and Apple Silicon (arm64)
// architectures, as well as universal binaries that run on both architectures.
//
// The Builder contains configuration options common between all macOS builds such as build directories,
// output directories, dry-run mode, etc.
type Builder struct {
	builddir  string // Directory where build artifacts are stored
	outputdir string // Directory where the final output will be placed
	dryRun    bool   // If true, perform a dry run without actual building

	builddirArm64     string // Directory for arm64 binaries
	builddirAmd64     string // Directory for amd64 binaries
	builddirUniversal string // Directory for universal binaries

	log *slog.Logger
}

// BuilderOpt is a functional option for configuring the Builder.
type BuilderOpt func(*Builder) error

// NewBuilder initializes a new TarballBuilder.
// It returns an error if the builder cannot be initialized.
func NewBuilder(opts ...BuilderOpt) (*Builder, error) {
	b := &Builder{
		log: slog.Default(),

		builddir:          "build",
		outputdir:         "output",
		builddirArm64:     filepath.Join("build", "darwin", "arm64"),
		builddirAmd64:     filepath.Join("build", "darwin", "amd64"),
		builddirUniversal: filepath.Join("build", "darwin", "universal"),
	}

	for _, o := range opts {
		if err := o(b); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return b, nil
}

// WithLogger sets the logger for the UniversalTarballBuilder.
func WithLogger(log *slog.Logger) BuilderOpt {
	return func(b *Builder) error {
		if log == nil {
			return errors.New("logger cannot be nil")
		}
		b.log = log
		return nil
	}
}

// WithDryRun sets the dry run mode for the Builder.
func WithDryRun(dryRun bool) BuilderOpt {
	return func(b *Builder) error {
		b.dryRun = dryRun
		return nil
	}
}

// WithBuildDir sets the build directory for the Builder.
func WithBuildDir(builddir string) BuilderOpt {
	return func(b *Builder) error {
		if builddir == "" {
			return errors.New("build directory cannot be empty")
		}
		b.builddir = builddir
		b.builddirArm64 = filepath.Join(builddir, "darwin", "arm64")
		b.builddirAmd64 = filepath.Join(builddir, "darwin", "amd64")
		b.builddirUniversal = filepath.Join(builddir, "darwin", "universal")
		return nil
	}
}

// WithOutputDir sets the output directory for the Builder.
func WithOutputDir(outputdir string) BuilderOpt {
	return func(b *Builder) error {
		if outputdir == "" {
			return errors.New("output directory cannot be empty")
		}
		b.outputdir = outputdir
		return nil
	}
}
