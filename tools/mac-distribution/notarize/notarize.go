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
	// maxRetries sets the max number of attempts for a succesful notarization
	maxRetries int

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

	// TeamID is the team identifier for the Apple Developer account.
	TeamID string
}

type Opt func(*Tool) error

// WithLogger sets the logger for the Tool.
func WithLogger(logger *slog.Logger) Opt {
	return func(t *Tool) error {
		t.log = logger
		return nil
	}
}

// MaxRetries sets the maximum number of retries for a successful notarization.
func MaxRetries(retries int) Opt {
	return func(t *Tool) error {
		t.maxRetries = retries
		return nil
	}
}

// DryRun sets the Tool to run in dry-run mode.
func DryRun() Opt {
	return func(t *Tool) error {
		t.dryRun = true
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
	MaxRetries(3),
}

func NewTool(creds Creds, opts ...Opt) (*Tool, error) {
	t := &Tool{
		Creds: creds,
	}
	for _, opt := range defaultOpts {
		if err := opt(t); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	for _, opt := range opts {
		if err := opt(t); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	t.cmdRunner = exec.NewDefaultCommandRunner()

	if t.dryRun {
		t.cmdRunner = exec.NewDryRunner(t.log)
		t.Creds.AppleUsername = "dryrun"
		t.Creds.ApplePassword = "dryrun"
		t.Creds.SigningIdentity = "dryrun"
		t.Creds.TeamID = "dryrun"
	}

	if err := t.validate(); err != nil {
		return nil, trace.Wrap(err)
	}

	return t, nil
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
	t.log.Info("codesigning binaries")
	out, err := t.runRetryable(func() ([]byte, error) {
		// Sometimes codesign can fail due to transient issues, so we retry
		// For example, we've seen "The timestamp service is not available."
		out, err := t.cmdRunner.RunCommand("codesign", args...)
		if err != nil {
			t.log.Error("failed to codesign binaries", "error", err)
		}
		return out, err
	})
	if err != nil {
		return trace.Wrap(err, "failed to codesign binaries: %v", out)
	}

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
	t.log.Info("successfully notarized binaries", "files", files)
	return nil
}

type AppBundleOpts struct {
	// Entitlements is the path to the entitlements file.
	// Entitlements are tied to a specific BundleID.
	Entitlements string
	// BundleID is a unique identifier for the package to be signed.
	// The codesign CLI doesn't normally require this but it will be infered if not present.
	// In an effort to make the process a bit more deterministic we will require it.
	// This is typically in reverse domain notation.
	// 		Example: com.gravitational.teleport.myapp
	BundleID string
}

// NotarizeAppBundle will notarize the app bundle located at the specified path.
func (t *Tool) NotarizeAppBundle(appBundlePath string, opts AppBundleOpts) error {
	if opts.BundleID == "" {
		return trace.BadParameter("Bundle ID is required to notarize app bundle")
	}
	// build args
	args := []string{
		"--sign", t.Creds.SigningIdentity,
		"--identifier", opts.BundleID,
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
	t.log.Info("codesigning app bundle")
	out, err := t.cmdRunner.RunCommand("codesign", args...)
	if err != nil {
		return trace.Wrap(err, "failed to codesign app bundle: %s", out)
	}

	// Zip the app bundle and submit for notarization
	notaryfile, err := os.CreateTemp("", fmt.Sprintf("notarize-*-%s.zip", filepath.Base(appBundlePath)))
	if err != nil {
		return trace.Wrap(err)
	}
	defer os.Remove(notaryfile.Name())
	t.log.Info("zipping app bundle", "zipfile", notaryfile.Name())
	if err := zipper.ZipDir(appBundlePath, notaryfile, zipper.IncludeParent()); err != nil {
		return trace.Wrap(err, "failed to zip app bundle")
	}

	if err := t.SubmitAndWait(notaryfile.Name()); err != nil {
		return trace.Wrap(err)
	}

	// Staple the app bundle
	t.log.Info("stapling app bundle")
	_, err = t.runRetryable(func() ([]byte, error) {
		out, err := t.cmdRunner.RunCommand("xcrun", "stapler", "staple", appBundlePath)
		if err != nil {
			t.log.Error("failed to staple package", "error", err)
		}
		return out, err
	})
	if err != nil {
		return trace.Wrap(err, "failed to staple package")
	}

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
	t.log.Info("stapling package", "package", pathToSigned)
	_, err = t.runRetryable(func() ([]byte, error) {
		out, err := t.cmdRunner.RunCommand("xcrun", "stapler", "staple", pathToSigned)
		if err != nil {
			t.log.Error("failed to staple package", "error", err)
		}
		return out, err
	})
	if err != nil {
		return trace.Wrap(err, "failed to staple package")
	}

	return nil
}

func (t *Tool) runRetryable(retryableFunc func() ([]byte, error)) ([]byte, error) {
	t.log.Info("attempting command", "attempt", 1)
	stdout, err := retryableFunc()
	for i := 0; err != nil && i < t.maxRetries; i += 1 {
		t.log.Info("retrying command", "attempt", i+2)
		stdout, err = retryableFunc()
	}
	return stdout, err
}

func (t *Tool) validate() error {
	missing := []string{}

	if t.Creds.AppleUsername == "" {
		missing = append(missing, "AppleUsername")
	}
	if t.Creds.ApplePassword == "" {
		missing = append(missing, "ApplePassword")
	}
	if t.Creds.SigningIdentity == "" {
		missing = append(missing, "SigningIdentity")
	}
	if t.Creds.TeamID == "" {
		missing = append(missing, "TeamID")
	}

	if len(missing) > 0 {
		return trace.BadParameter("missing required credentials: %v", missing)
	}

	return nil
}
