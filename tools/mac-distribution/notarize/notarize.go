package notarize

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/exec"
	"github.com/gravitational/shared-workflows/tools/mac-distribution/internal/zipper"

	"github.com/gravitational/trace"
)

// Tool is a wrapper around the MacOS codesigning/notarizing utilities.
type Tool struct {
	Creds Creds
	// Retry sets the max number of attempts for a succesful notarization
	retry int

	log *slog.Logger

	dryRun    bool
	cmdRunner exec.CommandRunner
}

// Creds contains the credentials needed to authenticate with the Apple Notary Service.
type Creds struct {
	// Credentials for authenticating with Apple Notary Service
	AppleUsername string
	ApplePassword string
	// SigningIdentity is the identity used to sign the package.
	// This is typically the Developer ID Application identity.
	SigningIdentity string
	// BundleID is a unique identifier for the package to be signed.
	// The codesign CLI doesn't normally require this but it will be infered if not present.
	// In an effort to make the process a bit more deterministic we will require it.
	// This is typically in reverse domain notation.
	// 		Example: com.gravitational.teleport.myapp
	BundleID string
}

type ToolOpts struct {
	Logger *slog.Logger
	Retry  int
	DryRun bool
}

func NewTool(creds Creds, opts ToolOpts) *Tool {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var runner exec.CommandRunner = exec.NewDefaultCommandRunner()
	if opts.DryRun {
		runner = exec.NewDryRunner(logger)
	}

	if creds.AppleUsername == "" && opts.DryRun {
		creds.AppleUsername = "dryrun"
	}

	if creds.ApplePassword == "" && opts.DryRun {
		creds.ApplePassword = "dryrun"
	}

	if creds.BundleID == "" && opts.DryRun {
		creds.BundleID = "dryrun"
	}

	if creds.SigningIdentity == "" && opts.DryRun {
		creds.SigningIdentity = "dryrun"
	}

	return &Tool{
		Creds:     creds,
		retry:     opts.Retry,
		log:       logger,
		cmdRunner: runner,
		dryRun:    opts.DryRun,
	}
}

// NotarizeBinaries will notarize the provided binaries.
// This will sign the binaries and submit them for notarization.
func (t *Tool) NotarizeBinaries(files []string) error {
	slices.Sort(files) // Sort the files for deterministic notarization

	// Codesign
	args := []string{
		"--sign", t.Creds.SigningIdentity,
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

	// Prepare zip files for notarization
	archiveFiles := []zipper.FileInfo{}
	for _, f := range files {
		archiveFiles = append(archiveFiles, zipper.FileInfo{Path: f})
	}

	notaryfile, err := os.CreateTemp("", "notarize-binaries-*.zip")
	if err != nil {
		return trace.Wrap(err)
	}
	defer notaryfile.Close()
	defer os.Remove(notaryfile.Name())

	t.log.Info("zipping binaries", "zipfile", notaryfile.Name())
	if err := zipper.ZipFiles(notaryfile, archiveFiles); err != nil {
		return trace.Wrap(err)
	}

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
		"--sign", t.Creds.SigningIdentity,
		"--identifier", t.Creds.BundleID,
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
	notaryfile, err := os.CreateTemp("", fmt.Sprintf("notarize-*-%s.zip", filepath.Base(appBundlePath)))
	if err != nil {
		return trace.Wrap(err)
	}
	t.log.Info("zipping app bundle", "zipfile", notaryfile.Name())
	if err := zipper.ZipDir(appBundlePath, notaryfile, zipper.IncludeParent()); err != nil {
		return trace.Wrap(err, "failed to zip app bundle")
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
		"--sign", t.Creds.SigningIdentity,
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
