/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// Runner executes a command, running any registered hooks at the appropriate time.
// Runner MUST be closed prior to program termination.
type Runner struct {
	hooks  []commandrunner.Hook
	logger *slog.Logger
}

// RunnerOption provides optional configuration to the runner.
type RunnerOption func(r *Runner)

// WithHooks adds hooks to the new runner.
func WithHooks(hooks ...commandrunner.Hook) RunnerOption {
	return func(r *Runner) {
		for _, hook := range hooks {
			if hook != nil {
				r.hooks = append(r.hooks, hook)
			}
		}
	}
}

// WithLogger sets the runner logger.
func WithLogger(logger *slog.Logger) RunnerOption {
	return func(r *Runner) {
		if logger == nil {
			logger = logging.DiscardLogger
		}
		r.logger = logger
	}
}

// NewRunner creates a new command runner.
func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		logger: logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Run executes the provided command. If setup hooks have not been successfully called, they
// will be ran prior to executing the command.
// Cleanup hooks will not be called explicitly. The caller should `Close` the runner after
// all associated command have been ran.
func (r *Runner) Run(ctx context.Context, name string, args ...string) error {

	// Build the command
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Allow hooks to modify the command (add env vars, args, etc.)
	for _, hook := range r.hooks {
		r.logger.DebugContext(ctx, "running command hook", "hook", hook.Name())
		if err := hook.Command(ctx, cmd); err != nil {
			return fmt.Errorf("hook %q command failed on command %q: %w", buildCommandDebugString(cmd),
				hook.Name(), err)
		}
	}

	cmdDebugString := buildCommandDebugString(cmd)
	r.logger.DebugContext(ctx, "running command", "command", cmdDebugString)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", cmdDebugString, err)
	}

	return nil
}

// buildCommandDebugString builds a textual version of cmd for debug logging.
// This should never be directly executed.
func buildCommandDebugString(cmd *exec.Cmd) string {
	// This is the same as `cmd.String()` but it includes env vars
	// Example: VAR1=val1 VAR2=val2 /path/to/command --with args
	return strings.Join(append(cmd.Env, cmd.String()), " ")
}
