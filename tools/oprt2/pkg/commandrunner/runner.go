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

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// Runner executes a command, running any registered hooks at the appropriate time.
// Runner MUST be closed prior to program termination.
type Runner struct {
	setupRan        bool
	setupHooks      []SetupHook
	preCommandHooks []PreCommandHook
	commandHooks    []CommandHook
	cleanupHooks    []CleanupHook
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

func NewRunner(opts ...RunnerOption) *Runner {
	r := &Runner{}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

func (r *Runner) RegisterHook(h Hook) {
	if h == nil {
		return
	}

	if setupHook, ok := h.(SetupHook); ok {
		r.setupHooks = append(r.setupHooks, setupHook)
	}

	if preCommandHook, ok := h.(PreCommandHook); ok {
		r.preCommandHooks = append(r.preCommandHooks, preCommandHook)
	}

	if commandHook, ok := h.(CommandHook); ok {
		r.commandHooks = append(r.commandHooks, commandHook)
	}

	if cleanupHook, ok := h.(CleanupHook); ok {
		r.cleanupHooks = append(r.cleanupHooks, cleanupHook)
	}
}

// Close runs all cleanup hooks. This should always run, even if the
// command errors.
func (r *Runner) Close(ctx context.Context) error {
	logger := logging.FromCtx(ctx)

	errs := make([]error, 0, len(r.cleanupHooks))
	for _, cleanupHook := range r.cleanupHooks {
		logger.DebugContext(ctx, "running cleanup hook", "hook", cleanupHook.Name())
		errs = append(errs, cleanupHook.Cleanup(ctx))
	}

	return errors.Join(errs...)
}

// Setup runs setup hooks. This will return after the first hook failure, or all hooks succeed.
func (r *Runner) Setup(ctx context.Context) error {
	logger := logging.FromCtx(ctx)

	for _, setupHook := range r.setupHooks {
		logger.DebugContext(ctx, "running setup hook", "hook", setupHook.Name())
		if err := setupHook.Setup(ctx); err != nil {
			return fmt.Errorf("setup hook %q failed: %w", setupHook.Name(), err)
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

	logger := logging.FromCtx(ctx)

	// Allow hooks to modify the command or arguments
	for _, preCommandHook := range r.preCommandHooks {
		logger.DebugContext(ctx, "running pre-command hook", "hook", preCommandHook.Name())
		if err := preCommandHook.PreCommand(ctx, &name, &args); err != nil {
			cmdString := buildCommandDebugString(exec.Command(name, args...))
			return fmt.Errorf("pre-command hook %q failed for command %q: %w",
				preCommandHook.Name(), cmdString, err)
		}
	}

	// Build the command
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Allow hooks to modify the command (add env vars, args, etc.)
	for _, commandHook := range r.commandHooks {
		logger.DebugContext(ctx, "running command hook", "hook", commandHook.Name())
		if err := commandHook.Command(ctx, cmd); err != nil {
			return fmt.Errorf("command hook %q failed on command %q: %w", buildCommandDebugString(cmd),
				commandHook.Name(), err)
		}
	}

	cmdDebugString := buildCommandDebugString(cmd)
	logger.DebugContext(ctx, "running command", "command", cmdDebugString)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", cmdDebugString, err)
	}

	return nil
}

// buildCommandDebugString builds a textual version of cmd for debugging.
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
