package notarize

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gravitational/trace"
)

// Tool is a wrapper around the MacOS codesigning/notarizing utilities.
type Tool struct {
	// Credentials for authenticating with Apple Notary Service
	AppleUsername string
	ApplePassword string

	// ApplicationIdentifier is the unique ID of the package being distributed.
	ApplicationIdentifier string

	// Entitlements is the path to the entitlements file.
	// Entitlements are tied to a specific ApplicationIdentifier.
	Entitlements string

	// SigningIdentity is the identity used to sign the package.
	// This is typically the Developer ID Application identity.
	SigningIdentity string

	// Retry sets the max number of attempts for a succesful notarization
	Retry int

	log slog.Logger

	cmdRunner commandRunner
}

func NewTool(appleUsername, applePassword, applicationIdentifier string, retry int, logger slog.Logger) *Tool {
	return &Tool{
		AppleUsername:         appleUsername,
		ApplePassword:         applePassword,
		ApplicationIdentifier: applicationIdentifier,
		Retry:                 retry,
		log:                   logger,
		cmdRunner:             &defaultCommandRunner{},
	}
}

func NewDryRunTool(logger slog.Logger) *Tool {
	return &Tool{
		log:       logger,
		cmdRunner: &defaultCommandRunner{},
	}
}

func (t *Tool) NotarizeBinaries(files []string) error {
	// Codesign
	args := []string{
		"--force",
		"--verbose",
		"--timestamp",
		"--options", "runtime",
	}
	args = append(args, files...)
	cmd := exec.Command("codesign", args...)

	if err := cmd.Run(); err != nil {
		return err
	}

	// Setup zip for notarization

	// Notarize
	// Stapling is not done for binaries
	if err := t.SubmitAndWait(""); err != nil {
		return trace.Wrap(err)
	}

	return cmd.Run()
}

// NotarizeAppBundle will notarize the app bundle located at the specified path.
func (t *Tool) NotarizeAppBundle(appBundlePath string) error {
	// build args
	args := []string{
		"--sign", t.SigningIdentity,
		"--identifier", t.ApplicationIdentifier,
		"--force",
		"--verbose",
		"--timestamp",
		"--options", "kill,hard,runtime",
	}

	if t.Entitlements != "" { // add entitlements if provided
		args = append(args, "--entitlements", t.Entitlements)
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
	zipper := &dirZipper{IncludePrefix: true}
	if err := zipper.ZipDir(appBundlePath, notaryfile); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (t *Tool) WithEntitlements(entitlements string) {
	t.Entitlements = entitlements
}

// commandWrapper is a wrapper around [exec.Command] that is useful for testing.
type commandRunner interface {
	RunCommand(path string, args ...string) (string, error)
}

type defaultCommandRunner struct{}

func (d *defaultCommandRunner) RunCommand(path string, args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(path, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(stderr.String()), trace.Wrap(err, "can't get last released version")
	}
	out := strings.TrimSpace(stdout.String())
	return out, nil
}
