package appbundle

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/fileutil"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
)

// Packager creates an app bundle (.app) for distribution.
type Packager struct {
	Info Info

	log        *slog.Logger
	notaryTool *notarize.Tool
	bundleID   string
}

// Info contains the information needed to create an app bundle.
type Info struct {
	// Skeleton directory to use as the base for the app bundle
	Skeleton string
	// Entitlements file to use for the app bundle
	Entitlements string
	// AppBinary is the binary to use as the main executable for the app bundle
	AppBinary string
}

// Opt is a functional option for configuring an AppBundle.
type Opt func(*Packager) error

// WithLogger sets the logger for the packager.
// By default, the packager will use slog.Default().
func WithLogger(log *slog.Logger) Opt {
	return func(a *Packager) error {
		a.log = log
		return nil
	}
}

// WithNotaryTool sets the notary tool for the packager.
func WithNotaryTool(tool *notarize.Tool) Opt {
	return func(a *Packager) error {
		a.notaryTool = tool
		return nil
	}
}

// WithBundleID sets the bundle ID  which is required for notarization of the app bundle.
func WithBundleID(bundleID string) Opt {
	return func(a *Packager) error {
		a.bundleID = bundleID
		return nil
	}
}

var defaultAppBundleOpts = []Opt{
	WithLogger(slog.Default()),
}

// NewAppBundlePackager creates a new AppBundlePackager.
func NewPackager(info Info, opts ...Opt) (*Packager, error) {
	if err := info.validate(); err != nil {
		return nil, err
	}
	app := &Packager{
		Info: info,
	}
	for _, opt := range defaultAppBundleOpts {
		if err := opt(app); err != nil {
			return nil, fmt.Errorf("applying default option: %w", err)
		}
	}

	for _, opt := range opts {
		if err := opt(app); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return app, nil
}

// Package creates an app bundle from the provided info.
func (a *Packager) Package() error {
	// Copy the app binary into the skeleton
	binDir := filepath.Join(a.Info.Skeleton, "Contents", "MacOS")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	binDest := filepath.Join(binDir, filepath.Base(a.Info.AppBinary))
	if err := fileutil.CopyFile(a.Info.AppBinary, binDest, fileutil.WithDestPermissions(0755)); err != nil {
		return err
	}

	if a.notaryTool == nil {
		a.log.Info("notarization skipped")
		return nil
	}

	// Notarize the app bundle
	if err := a.notaryTool.NotarizeAppBundle(a.Info.Skeleton, notarize.AppBundleOpts{Entitlements: a.Info.Entitlements, BundleID: a.bundleID}); err != nil {
		return err
	}

	a.log.Info("successfully created app bundle", "path", a.Info.Skeleton)
	return nil
}

func (a *Info) validate() error {
	// Validate skeleton
	info, err := os.Stat(a.Skeleton)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("skeleton directory %q does not exist", a.Skeleton)
		}
		return fmt.Errorf("stat skeleton directory %q: %w", a.Skeleton, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("skeleton %q must be a directory", a.Skeleton)
	}

	if filepath.Ext(a.Skeleton) != ".app" {
		return fmt.Errorf("skeleton %q must have .app extension", a.Skeleton)
	}

	// Validate binary
	info, err = os.Stat(a.AppBinary)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("app binary %q does not exist", a.AppBinary)
		}
		return fmt.Errorf("stat app binary %q: %w", a.AppBinary, err)
	}

	if info.IsDir() {
		return fmt.Errorf("app binary %q must be a file", a.AppBinary)
	}

	// todo: consider validating binary is signed

	return nil
}
