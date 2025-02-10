package exec

import (
	"bytes"
	"log/slog"
	"os/exec"

	"github.com/gravitational/trace"
)

// CommandRunner is a wrapper around [exec.Command] that is useful for testing.
type CommandRunner interface {
	RunCommand(path string, args ...string) ([]byte, error)
}

func NewDefaultCommandRunner() *DefaultCommandRunner {
	return &DefaultCommandRunner{}
}

type DefaultCommandRunner struct {
}

var _ CommandRunner = &DefaultCommandRunner{}

func (d *DefaultCommandRunner) RunCommand(path string, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(path, args...)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err := cmd.Run()
	out := bytes.TrimSpace(stdout.Bytes())
	if err != nil {
		// stdout is also returned since it may contain useful information
		return out, trace.Wrap(err, "failed to run command: %s", stderr.String())
	}
	return out, nil
}

// DryRunner is a dry runner that does not actually run the command.
// Instead, it logs the command that would have been run.
type DryRunner struct {
	log *slog.Logger
}

var _ CommandRunner = &DryRunner{}

// NewDryRunner creates a new dry runner.
func NewDryRunner(logger *slog.Logger) *DryRunner {
	return &DryRunner{
		log: logger,
	}
}

// RunCommand logs the command that would have been run.
func (d *DryRunner) RunCommand(path string, args ...string) ([]byte, error) {
	d.log.Info("dry run", "path", path, "args", args)
	return []byte("dry run"), nil
}
