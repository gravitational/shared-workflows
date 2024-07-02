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

// RunCmd runs the GitHub CLI returns output (stdout/stderr, depends on the cmd result) and error
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
