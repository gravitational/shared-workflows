package packaging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/notarize"
	"github.com/gravitational/trace"
)

// AppBundle represents an app bundle to be packaged for distribution.
type AppBundle struct {
	// Skeleton directory to use as the base for the app bundle
	Skeleton string
	// Entitlements file to use for the app bundle
	Entitlements string
	// AppBinary is the binary to use as the main executable for the app bundle
	AppBinary string
	// NotarizationEnabled determines whether to notarize the app bundle
	NotaryTool *notarize.Tool
}

func NewAppBundle(skeleton, entitlements, appBinary string) *AppBundle {
	return &AppBundle{
		Skeleton:     skeleton,
		Entitlements: entitlements,
		AppBinary:    appBinary,
	}
}

func (a *AppBundle) Build() error {
	if err := a.validate(); err != nil {
		return trace.Wrap(err)
	}

	// Copy the app binary into the skeleton
	binDir := filepath.Join(a.Skeleton, "Contents", "MacOS")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return trace.Wrap(err)
	}

	binDest := filepath.Join(binDir, filepath.Base(a.AppBinary))
	w, err := os.OpenFile(binDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755) // create or overwrite
	if err != nil {
		return trace.Wrap(err)
	}
	defer w.Close()

	r, err := os.OpenFile(a.AppBinary, os.O_RDONLY, 0)
	defer r.Close()
	if err != nil {
		return trace.Wrap(err)
	}
	if _, err = io.Copy(w, r); err != nil {
		return trace.Wrap(err)
	}

	// Notarize the app bundle
	a.NotaryTool.WithEntitlements(a.Entitlements)
	return trace.Wrap(a.NotaryTool.NotarizeAppBundle(a.Skeleton))
}

func (a *AppBundle) validate() error {
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
