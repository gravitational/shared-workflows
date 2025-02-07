package packaging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/fileutil"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/trace"
)

// AppBundlePackager is a packager for creating an app bundle (.app) for distribution.
type AppBundlePackager struct {
	Info *AppBundleInfo

	log        *slog.Logger
	notaryTool *notarize.Tool
}

// AppBundleInfo represents an app bundle to be packaged for distribution.
type AppBundleInfo struct {
	// Skeleton directory to use as the base for the app bundle
	Skeleton string
	// Entitlements file to use for the app bundle
	Entitlements string
	// AppBinary is the binary to use as the main executable for the app bundle
	AppBinary string
}

// AppBundlePackagerOpts contains options for creating an AppBundlePackager.
type AppBundlePackagerOpts struct {
	// NotarizationEnabled determines whether to notarize the app bundle
	NotaryTool *notarize.Tool
	// Logger is the logger to use for the packager
	Logger *slog.Logger
}

// NewAppBundlePackager creates a new AppBundlePackager.
func NewAppBundlePackager(info *AppBundleInfo, opts *AppBundlePackagerOpts) *AppBundlePackager {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	return &AppBundlePackager{
		Info:       info,
		log:        log,
		notaryTool: opts.NotaryTool,
	}
}

// Package creates an app bundle from the provided info.
func (a *AppBundlePackager) Package() error {
	if err := a.Info.validate(); err != nil {
		return trace.Wrap(err)
	}

	// Copy the app binary into the skeleton
	binDir := filepath.Join(a.Info.Skeleton, "Contents", "MacOS")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return trace.Wrap(err)
	}

	binDest := filepath.Join(binDir, filepath.Base(a.Info.AppBinary))
	if err := fileutil.CopyFile(a.Info.AppBinary, binDest); err != nil {
		return trace.Wrap(err)
	}

	if a.notaryTool == nil {
		a.log.Info("notarization skipped")
		return nil
	}

	// Notarize the app bundle
	if err := a.notaryTool.NotarizeAppBundle(a.Info.Skeleton, notarize.AppBundleOpts{Entitlements: a.Info.Entitlements}); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (a *AppBundleInfo) validate() error {
	// Validate skeleton
	info, err := os.Stat(a.Skeleton)
	if err != nil {
		return trace.Wrap(err, "skeleton directory does not exist")
	}

	if !info.IsDir() {
		return fmt.Errorf("skeleton must be a directory")
	}

	if filepath.Ext(a.Skeleton) != ".app" {
		return fmt.Errorf("skeleton directory must have .app extension")
	}

	// Validate binary
	info, err = os.Stat(a.AppBinary)
	if err != nil {
		return trace.Wrap(err, "app binary does not exist")
	}

	if info.IsDir() {
		return fmt.Errorf("app binary must be a file")
	}

	// todo: consider validating binary is signed

	return nil
}
