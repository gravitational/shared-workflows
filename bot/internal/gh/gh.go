package gh

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/gravitational/trace"
)

// IsAvailable returns status of git
func IsAvailable() error {
	_, err := exec.LookPath("gh")
	return err
}

// RunCmd runs git and returns output (stdout/stderr, depends on the cmd result) and error
func RunCmd(dir string, args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("gh", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = dir

	err := cmd.Run()

	if err != nil {
		return strings.TrimSpace(stderr.String()), trace.Wrap(err)
	}

	return strings.TrimSpace(stdout.String()), nil
}
