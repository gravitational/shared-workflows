package darwin

import (
	"errors"
	"fmt"
	"log/slog"
)

// UnversalTarballBuilder builds a tairball that contains multi-architecture binaries for macOS.
// The "universal" binaries can run on both Intel and Apple Silicon Macs.
// This is useful for distributing a single tarball that works across different Mac architectures.
//
// This uses the `lipo` tool to create a universal binary from multiple architecture-specific binaries.
// The best way to find more information on `lipo` is to refer to the official Apple documentation or run `man lipo` in the terminal.
type UniversalTarballBuilder struct {
	log *slog.Logger
}

// UniversalTarballBuilderOpt is a functional option for configuring the UniversalTarballBuilder.
type UniversalTarballBuilderOpt func(*UniversalTarballBuilder) error

// NewUniversalTarballBuilder initializes a new UniversalTarballBuilder.
// It returns an error if the builder cannot be initialized.
func NewUniversalTarballBuilder(opts ...UniversalTarballBuilderOpt) (*UniversalTarballBuilder, error) {
	b := &UniversalTarballBuilder{
		log: slog.Default(),
	}

	for _, o := range opts {
		if err := o(b); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return b, nil
}

// Build creates a universal tarball for macOS that contains binaries for both Intel and Apple Silicon architectures.
func (b *UniversalTarballBuilder) Build() error {
	return errors.New("building universal tarball for darwin is not implemented yet")
}

// WithLogger sets the logger for the UniversalTarballBuilder.
func (b *UniversalTarballBuilder) WithLogger(log *slog.Logger) UniversalTarballBuilderOpt {
	return func(b *UniversalTarballBuilder) error {
		if log == nil {
			return errors.New("logger cannot be nil")
		}
		b.log = log
		return nil
	}
}
