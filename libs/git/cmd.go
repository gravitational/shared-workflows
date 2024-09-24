/*
Copyright 2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package git

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/gravitational/trace"
)

// IsAvailable returns status of git
func IsAvailable() error {
	_, err := exec.LookPath("git")
	return err
}

// RunCmd runs git and returns output (stdout/stderr, depends on the cmd result) and error
func (r *Repo) RunCmd(args ...string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Dir = r.dir

	err := r.runner.Run(cmd)

	if err != nil {
		return strings.TrimSpace(stderr.String()), trace.Wrap(err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// commandRunner is a small interface that wraps the actual execution of the process.
// This allows us to easily swap out the implementation for testing.
type commandRunner interface {
	Run(*exec.Cmd) error
}

type defaultRunner struct{}

func (r *defaultRunner) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}
