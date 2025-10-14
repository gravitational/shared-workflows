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

package commandrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// Runner executes a command, running any registered hooks at the appropriate time.
// Runner MUST be closed prior to program termination.
type Runner struct {
	setupRan bool
	hooks    []Hook
	logger   *slog.Logger
}

type RunnerOption func(r *Runner)

// WithHooks adds hooks to the new runner.
func WithHooks(hooks ...Hook) RunnerOption {
	return func(r *Runner) {
		for _, hook := range hooks {
			r.RegisterHook(hook)
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

func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{
		logger: logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *Runner) RegisterHook(h Hook) {
	if h == nil {
		return
	}

	r.hooks = append(r.hooks, h)
}

// Close runs all cleanup hooks. This should always run, even if the
// command errors.
func (r *Runner) Close(ctx context.Context) error {
	errs := make([]error, 0, len(r.hooks))
	for _, hook := range r.hooks {
		r.logger.DebugContext(ctx, "running hook cleanup", "hook", hook.Name())
		errs = append(errs, hook.Cleanup(ctx))
	}

	return errors.Join(errs...)
}

// Setup runs setup hooks. This will return after the first hook failure, or all hooks succeed.
func (r *Runner) Setup(ctx context.Context) error {
	for _, hook := range r.hooks {
		r.logger.DebugContext(ctx, "running hook setup", "hook", hook.Name())
		if err := hook.Setup(ctx); err != nil {
			return fmt.Errorf("hook %q setup failed: %w", hook.Name(), err)
		}
	}

	r.setupRan = true
	return nil
}

// Run executes the provided command. If setup hooks have not been successfully called, they
// will be ran prior to executing the command.
// Cleanup hooks will not be called explicitly. The caller should `Close` the runner after
// all associated command have been ran.
func (r *Runner) Run(ctx context.Context, name string, args ...string) error {
	// Ensure setup has ran successfully at least once
	if !r.setupRan {
		if err := r.Setup(ctx); err != nil {
			return fmt.Errorf("setup failed: %w", err)
		}
	}

	// Allow hooks to modify the command or arguments
	for _, hook := range r.hooks {
		r.logger.DebugContext(ctx, "running hook pre-command", "hook", hook.Name())
		if err := hook.PreCommand(ctx, &name, &args); err != nil {
			cmdString := buildCommandDebugString(exec.Command(name, args...))
			return fmt.Errorf("hook %q pre-command failed for command %q: %w",
				hook.Name(), cmdString, err)
		}
	}

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
	if cmd == nil {
		return ""
	}

	// This is the same as `cmd.String()` but it includes env vars
	// Example: VAR1=val1 VAR2=val2 /path/to/command --with args
	commandTextBuilder := new(strings.Builder)
	for _, envVar := range cmd.Env {
		// The current implementation will never return a a non-nil error, and
		// handling one would just complicate this logic without much added value
		_, _ = commandTextBuilder.WriteString(envVar)
		_, _ = commandTextBuilder.WriteRune(' ')
	}
	commandTextBuilder.WriteString(cmd.String())

	return commandTextBuilder.String()
}
