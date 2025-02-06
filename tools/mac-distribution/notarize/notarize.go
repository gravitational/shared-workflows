package notarize

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/exec"

	"github.com/gravitational/trace"
)

// Tool is a wrapper around the MacOS codesigning/notarizing utilities.
type Tool struct {
	// Credentials for authenticating with Apple Notary Service
	AppleUsername string
	ApplePassword string

	// BundleID is the unique ID of the package being distributed.
	BundleID string

	// SigningIdentity is the identity used to sign the package.
	// This is typically the Developer ID Application identity.
	SigningIdentity string

	// Retry sets the max number of attempts for a succesful notarization
	Retry int

	log *slog.Logger

	dryRun    bool
	cmdRunner exec.CommandRunner
	zipper    zipper
}

type ToolOpts struct {
	Logger *slog.Logger
	Retry  int
	DryRun bool
}

func NewTool(appleUsername, applePassword, bundleID string, opts ToolOpts) *Tool {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var runner exec.CommandRunner = &exec.DefaultCommandRunner{}
	if opts.DryRun {
		runner = exec.NewDryRunner(logger)
	}

	if appleUsername == "" && opts.DryRun {
		appleUsername = "dryrun"
	}

	if applePassword == "" && opts.DryRun {
		applePassword = "dryrun"
	}

	if bundleID == "" && opts.DryRun {
		bundleID = "dryrun"
	}

	return &Tool{
		AppleUsername: appleUsername,
		ApplePassword: applePassword,
		BundleID:      bundleID,
		Retry:         opts.Retry,
		log:           logger,
		cmdRunner:     runner,
		zipper:        &defaultZipper{},
		dryRun:        opts.DryRun,
	}
}

// NotarizeBinaries will notarize the provided binaries.
// This will sign the binaries and submit them for notarization.
func (t *Tool) NotarizeBinaries(files []string) error {
	// Codesign
	args := []string{
		"--force",
		"--verbose",
		"--timestamp",
		"--options", "runtime",
	}
	args = append(args, files...)
	out, err := t.cmdRunner.RunCommand("codesign", args...)
	if err != nil {
		return trace.Wrap(err, "failed to codesign binaries: %v", out)
	}
	t.log.Info("codesign output", "output", out)

	// The Apple Notary Service requires a zip file for notarization.
	// The files will be staged in a temporary directory and zipped for submission.
	notaryDir, err := os.MkdirTemp("", "notarize-binaries-*")
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.RemoveAll(notaryDir)

	// Stage files for notarization
	for _, file := range files {
		dest := filepath.Join(notaryDir, filepath.Base(file))
		r, err := os.OpenFile(file, os.O_RDONLY, 0)
		if err != nil {
			return trace.Wrap(err)
		}
		defer r.Close()

		w, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return trace.Wrap(err)
		}
		defer w.Close()

		if _, err := io.Copy(w, r); err != nil {
			return trace.Wrap(err)
		}
	}

	// Zip the staged directory where the binaries are located
	notaryfile, err := os.CreateTemp("", "notarize-binaries-*.zip")
	if err != nil {
		return trace.Wrap(err)
	}
	defer notaryfile.Close()
	defer os.Remove(notaryfile.Name())

	if err := t.zipper.ZipDir(notaryDir, notaryfile, zipDirOpts{}); err != nil {
		return trace.Wrap(err)
	}

	// Notarize
	if err := t.SubmitAndWait(notaryfile.Name()); err != nil {
		return trace.Wrap(err)
	}
	// Stapling is not done for binaries
	return nil
}

type AppBundleOpts struct {
	// Entitlements is the path to the entitlements file.
	// Entitlements are tied to a specific BundleID.
	Entitlements string
}

// NotarizeAppBundle will notarize the app bundle located at the specified path.
func (t *Tool) NotarizeAppBundle(appBundlePath string, opts AppBundleOpts) error {
	// build args
	args := []string{
		"--sign", t.SigningIdentity,
		"--identifier", t.BundleID,
		"--force",
		"--verbose",
		"--timestamp",
		"--options", "kill,hard,runtime",
	}

	if opts.Entitlements != "" { // add entitlements if provided
		args = append(args, "--entitlements", opts.Entitlements)
	}

	// codesign the app bundle
	args = append(args, appBundlePath)
	out, err := t.cmdRunner.RunCommand("codesign", args...)
	if err != nil {
		return trace.Wrap(err, "failed to codesign app bundle: %v", out)
	}

	t.log.Info("codesign output", "output", out)

	// Zip the app bundle and submit for notarization
	notaryfile, err := os.CreateTemp("", "notarize-app-bundle-"+filepath.Base(appBundlePath))
	if err != nil {
		return trace.Wrap(err)
	}
	if err := t.zipper.ZipDir(appBundlePath, notaryfile, zipDirOpts{IncludePrefix: true}); err != nil {
		return trace.Wrap(err)
	}

	// Staple the app bundle
	out, err = t.cmdRunner.RunCommand("xcrun", "stapler", "staple", appBundlePath)
	if err != nil {
		return trace.Wrap(err, "failed to staple package")
	}
	t.log.Info("stapler output", "output", out)

	return nil
}

// NotarizePackageInstaller will notarize a given package installer (.pkg).
// Signing the package installer creates a new file for the signed package.
func (t *Tool) NotarizePackageInstaller(pathToUnsigned, pathToSigned string) error {
	// Productsign
	args := []string{
		"--sign", t.SigningIdentity,
		"--timestamp",
		pathToUnsigned,
		pathToSigned,
	}

	out, err := t.cmdRunner.RunCommand("productsign", args...)
	if err != nil {
		return trace.Wrap(err, "failed to productsign package")
	}
	t.log.Info("productsign output", "output", out)

	// Notarize
	if err := t.SubmitAndWait(pathToSigned); err != nil {
		return trace.Wrap(err)
	}

	// Staple
	out, err = t.cmdRunner.RunCommand("xcrun", "stapler", "staple", pathToSigned)
	if err != nil {
		return trace.Wrap(err, "failed to staple package")
	}
	t.log.Info("stapler output", "output", out)

	return nil
}
