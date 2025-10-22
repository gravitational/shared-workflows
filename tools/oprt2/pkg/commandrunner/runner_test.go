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
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestWithHooks(t *testing.T) {
	testHookA := &mockHook{}
	testHookB := &mockHook{}

	tests := []struct {
		name          string
		existingHooks []Hook
		providedHooks []Hook
		expectedHooks []Hook
	}{
		{
			name: "no hooks",
		},
		{
			name: "no new hooks",
			existingHooks: []Hook{
				testHookA,
				testHookB,
			},
			expectedHooks: []Hook{
				testHookA,
				testHookB,
			},
		},
		{
			name: "new hook",
			existingHooks: []Hook{
				testHookA,
				testHookB,
			},
			providedHooks: []Hook{
				testHookA,
			},
			expectedHooks: []Hook{
				testHookA,
				testHookB,
				testHookA,
			},
		},
		{
			name: "new hooks",
			existingHooks: []Hook{
				testHookA,
				testHookB,
			},
			providedHooks: []Hook{
				testHookA,
				testHookB,
				testHookA,
			},
			expectedHooks: []Hook{
				testHookA,
				testHookB,
				testHookA,
				testHookB,
				testHookA,
			},
		},
		{
			name: "nil hook",
			providedHooks: []Hook{
				nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Runner{
				hooks: tt.existingHooks,
			}

			opt := WithHooks(tt.providedHooks...)
			opt(runner)

			assert.EqualValues(t, tt.expectedHooks, runner.hooks)
		})
	}
}

func TestWithLogger(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name           string
		providedLogger *slog.Logger
		expectedLogger *slog.Logger
	}{
		{
			name:           "with nil logger",
			expectedLogger: logging.DiscardLogger,
		},
		{
			name:           "with new logger",
			providedLogger: testLogger,
			expectedLogger: testLogger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Runner{}

			opt := WithLogger(tt.providedLogger)
			opt(runner)

			assert.EqualValues(t, tt.expectedLogger, runner.logger)
		})
	}
}

func TestNewRunner(t *testing.T) {
	commonTests := func(t *testing.T, r *Runner) {
		require.NotNil(t, r)
		assert.False(t, r.setupRan)
		assert.Empty(t, r.hooks)
		assert.Equal(t, r.logger, logging.DiscardLogger)
	}

	t.Run("basic runner", func(t *testing.T) {
		commonTests(t, NewRunner())
	})

	t.Run("runner with options", func(t *testing.T) {
		calledA := false
		optionA := func(r *Runner) {
			calledA = true
			assert.NotNil(t, r)
		}

		calledB := false
		optionB := func(r *Runner) {
			calledB = true
			assert.True(t, calledA) // Check call order
			assert.NotNil(t, r)
		}

		r := NewRunner(optionA, optionB)

		commonTests(t, r)
		assert.True(t, calledB)
	})
}

func TestRegisterHook(t *testing.T) {
	testHookA := &mockHook{}
	testHookB := &mockHook{}

	tests := []struct {
		name          string
		runner        *Runner
		providedHook  Hook
		expectedHooks []Hook
		errFunc       assert.ErrorAssertionFunc
	}{
		{
			name:         "new hook",
			runner:       &Runner{},
			providedHook: testHookA,
			expectedHooks: []Hook{
				testHookA,
			},
		},
		{
			name: "new hook with existing hooks",
			runner: &Runner{
				hooks: []Hook{
					testHookA,
					testHookB,
				},
			},
			providedHook: testHookA,
			expectedHooks: []Hook{
				testHookA,
				testHookB,
				testHookA,
			},
		},
		{
			name: "nil hook",
			runner: &Runner{
				hooks: []Hook{
					testHookA,
					testHookB,
				},
			},
			expectedHooks: []Hook{
				testHookA,
				testHookB,
			},
		},
		{
			name: "setup already ran",
			runner: &Runner{
				hooks: []Hook{
					testHookA,
					testHookB,
				},
				setupRan: true,
			},
			providedHook: testHookA,
			expectedHooks: []Hook{
				testHookA,
				testHookB,
			},
			errFunc: assert.Error,
		},
		{
			name: "setup already ran but nil hook",
			runner: &Runner{
				hooks: []Hook{
					testHookA,
					testHookB,
				},
				setupRan: true,
			},
			expectedHooks: []Hook{
				testHookA,
				testHookB,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = assert.NoError
			}

			err := tt.runner.RegisterHook(tt.providedHook)

			tt.errFunc(t, err)
			assert.EqualValues(t, tt.expectedHooks, tt.runner.hooks)
		})
	}
}

func TestClose(t *testing.T) {
	buildHook := func(name string, err error) *mockHook {
		h := &mockHook{}
		h.On("Cleanup", mock.Anything).Once().Return(err)
		h.On("Name", mock.Anything).Return(name)

		return h
	}

	t.Run("no hooks", func(t *testing.T) {
		assert.NoError(t, NewRunner().Close(t.Context()))
	})

	t.Run("with hooks", func(t *testing.T) {
		ctx := t.Context()

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Close(ctx)

		assert.NoError(t, err)
		hookA.AssertCalled(t, "Cleanup", ctx)
		hookB.AssertCalled(t, "Cleanup", ctx)
	})

	t.Run("with hooks and cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Close(ctx)

		assert.NoError(t, err)
		// Hooks should still be called and they can choose whether or not
		// to ignore the cancellation
		hookA.AssertCalled(t, "Cleanup", ctx)
		hookB.AssertCalled(t, "Cleanup", ctx)
	})

	t.Run("with hooks with error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		hookA := buildHook("Hook A", assert.AnError)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Close(ctx)

		assert.Error(t, err)
		hookA.AssertCalled(t, "Cleanup", ctx)
		// Even if a previous hook errors, later hooks should still be called
		hookB.AssertCalled(t, "Cleanup", ctx)
	})

	t.Run("with hooks with multiple errors", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		hookA := buildHook("Hook A", assert.AnError)
		hookB := buildHook("Hook B", assert.AnError)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Close(ctx)

		require.Error(t, err)

		errs, ok := err.(interface{ Unwrap() []error })
		require.True(t, ok, "provided error does not support unwrapping multiple errors")
		assert.Equal(t, errs.Unwrap(), []error{assert.AnError, assert.AnError})

		hookA.AssertCalled(t, "Cleanup", ctx)
		hookB.AssertCalled(t, "Cleanup", ctx)
	})
}

func TestSetup(t *testing.T) {
	buildHook := func(name string, err error) *mockHook {
		h := &mockHook{}
		h.On("Setup", mock.Anything).Once().Return(err)
		h.On("Name").Return(name)

		return h
	}

	t.Run("no hooks", func(t *testing.T) {
		assert.NoError(t, NewRunner().Setup(t.Context()))
	})

	t.Run("with hooks", func(t *testing.T) {
		ctx := t.Context()

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Setup(ctx)

		assert.NoError(t, err)
		hookA.AssertCalled(t, "Setup", ctx)
		hookB.AssertCalled(t, "Setup", ctx)
		assert.True(t, r.setupRan)
	})

	t.Run("with hooks and setup already ran", func(t *testing.T) {
		ctx := t.Context()

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))
		r.setupRan = true

		err := r.Setup(ctx)

		assert.NoError(t, err)
		hookA.AssertNotCalled(t, "Setup", ctx)
		hookB.AssertNotCalled(t, "Setup", ctx)
		assert.True(t, r.setupRan)
	})

	t.Run("with hooks with error", func(t *testing.T) {
		ctx := t.Context()

		hookA := buildHook("Hook A", assert.AnError)
		hookB := buildHook("Hook B", nil)

		r := NewRunner(WithHooks(hookA, hookB))

		err := r.Setup(ctx)

		assert.Error(t, err)
		hookA.AssertCalled(t, "Setup", ctx)
		// Should fail on first error
		hookB.AssertNotCalled(t, "Setup", ctx)
		// Should not mark as setup ran if an error occured
		assert.False(t, r.setupRan)
	})
}

func TestRun(t *testing.T) {
	t.Run("basic no-op", func(t *testing.T) {
		assert.NoError(t, NewRunner().Run(t.Context(), "true"))
	})

	t.Run("setup hooks called if not already ran", func(t *testing.T) {
		ctx := t.Context()

		h := &mockHook{}
		h.On("Name").Maybe().Return("Test hook")
		h.On("Setup", ctx).Once().Return(assert.AnError)

		r := NewRunner(WithHooks(h))
		err := r.Run(ctx, "echo", "test")
		assert.Error(t, err)

		h.AssertCalled(t, "Setup", ctx)
	})

	t.Run("setup hooks not called again if already ran", func(t *testing.T) {
		ctx := t.Context()

		h := &mockHook{}
		h.On("Name").Maybe().Return("Test hook")
		h.On("PreCommand", ctx, mock.Anything, mock.Anything).Return(assert.AnError)

		r := NewRunner(WithHooks(h))
		r.setupRan = true

		err := r.Run(ctx, "echo", "test")
		assert.Error(t, err)

		h.AssertNotCalled(t, "Setup", ctx)
	})

	t.Run("multiple precommand hooks with one failing", func(t *testing.T) {
		ctx := t.Context()

		buildHook := func(name string, err error) *mockHook {
			h := &mockHook{}
			h.On("Name").Maybe().Return(name)
			h.On("Setup", ctx).Once().Return(nil)
			h.On("PreCommand", ctx, mock.Anything, mock.Anything).Once().Return(err).Run(func(args mock.Arguments) {
				// Validate command parameters
				cmd, ok := args.Get(1).(*string)
				require.True(t, ok)
				require.NotNil(t, cmd)
				assert.Equal(t, "echo", *cmd)

				cmdArgs, ok := args.Get(2).(*[]string)
				require.True(t, ok)
				require.NotNil(t, cmdArgs)
				assert.Equal(t, *cmdArgs, []string{"test"})
			})

			return h
		}

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", assert.AnError)
		hookC := buildHook("Hook C", nil)

		r := NewRunner(WithHooks(hookA, hookB, hookC))
		err := r.Run(ctx, "echo", "test")
		assert.Error(t, err)

		hookA.AssertCalled(t, "PreCommand", ctx, mock.Anything, mock.Anything)
		hookB.AssertCalled(t, "PreCommand", ctx, mock.Anything, mock.Anything)
		hookC.AssertNotCalled(t, "PreCommand", ctx, mock.Anything, mock.Anything)
	})

	t.Run("multiple command hooks with one failing", func(t *testing.T) {
		ctx := t.Context()

		buildHook := func(name string, err error) *mockHook {
			h := &mockHook{}
			h.On("Name").Maybe().Return(name)
			h.On("Setup", ctx).Once().Return(nil)
			h.On("PreCommand", ctx, mock.Anything, mock.Anything).Once().Return(nil)
			h.On("Command", ctx, mock.Anything).Once().Return(err).Run(func(args mock.Arguments) {
				// Validate command parameters
				cmd, ok := args.Get(1).(*exec.Cmd)
				require.True(t, ok)
				require.NotNil(t, cmd)

				assert.Equal(t, []string{"echo", "test"}, cmd.Args)
				assert.Equal(t, os.Stdout, cmd.Stdout)
				assert.Equal(t, os.Stderr, cmd.Stderr)
				assert.Equal(t, os.Stdin, cmd.Stdin)
			})

			return h
		}

		hookA := buildHook("Hook A", nil)
		hookB := buildHook("Hook B", assert.AnError)
		hookC := buildHook("Hook C", nil)

		r := NewRunner(WithHooks(hookA, hookB, hookC))
		err := r.Run(ctx, "echo", "test")
		assert.Error(t, err)

		hookA.AssertCalled(t, "Command", ctx, mock.Anything, mock.Anything)
		hookB.AssertCalled(t, "Command", ctx, mock.Anything, mock.Anything)
		hookC.AssertNotCalled(t, "Command", ctx, mock.Anything, mock.Anything)
	})

	t.Run("full test", func(t *testing.T) {
		ctx := t.Context()

		var stdoutCapture bytes.Buffer

		h := &mockHook{}
		setupFunc := h.On("Setup", ctx).Once().Return(nil)
		preCmdFunc := h.On("PreCommand", ctx, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			// Validate command parameters
			cmd, ok := args.Get(1).(*string)
			require.True(t, ok)
			require.NotNil(t, cmd)
			assert.Equal(t, "echo", *cmd)

			cmdArgs, ok := args.Get(2).(*[]string)
			require.True(t, ok)
			require.NotNil(t, cmdArgs)
			assert.Equal(t, *cmdArgs, []string{"base arg"})

			// Change the parameters
			*cmd = "sh"
			*cmdArgs = []string{"-c", "printf 'arg replaced by PreCommand'"}
		})
		cmdFunc := h.On("Command", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			// Valiate the command parameters
			cmd, ok := args.Get(1).(*exec.Cmd)
			require.True(t, ok)
			require.NotNil(t, cmd)

			assert.Equal(t, "sh", filepath.Base(cmd.Path))
			assert.Equal(t, []string{"sh", "-c", "printf 'arg replaced by PreCommand'"}, cmd.Args)

			// Change the parameters
			cmd.Args = []string{"sh", "-c", "printf 'env var: %s' \"${TEST_VAR}\""}
			cmd.Env = append(cmd.Env, "TEST_VAR=test value")
			cmd.Stdout = &stdoutCapture
		}).Twice()

		h.On("Name").Return("Test hook")
		cmdFunc.NotBefore(preCmdFunc.NotBefore(setupFunc))

		r := NewRunner(WithHooks(h))

		// Run a command once
		assert.NoError(t, r.Run(ctx, "echo", "base arg"))
		assert.Equal(t, "env var: test value", stdoutCapture.String())
		stdoutCapture.Truncate(0)

		// Run a command again
		assert.NoError(t, r.Run(ctx, "echo", "base arg"))
		assert.Equal(t, "env var: test value", stdoutCapture.String())
	})

	t.Run("with failing process", func(t *testing.T) {
		r := NewRunner()
		assert.Error(t, r.Run(t.Context(), "false"))
	})
}

func TestBuildCommandDebugString(t *testing.T) {
	echoPath, err := exec.LookPath("echo")
	assert.NoError(t, err, "failed to look up 'echo' path for test cases")

	tests := []struct {
		name           string
		command        string
		args           []string
		envVars        []string
		expectedOutput string
	}{
		{
			name:           "basic command",
			command:        "/some/command/path",
			expectedOutput: "/some/command/path",
		},
		{
			name:           "non-absolute path command",
			command:        "echo",
			expectedOutput: echoPath,
		},
		{
			name:           "with args",
			command:        "/some/command/path",
			expectedOutput: "/some/command/path arg1 arg2 arg3",
			args:           []string{"arg1", "arg2", "arg3"},
		},
		{
			name:           "with args and env vars",
			command:        "/some/command/path",
			expectedOutput: "KEY1=VALUE1 KEY2=VALUE2 /some/command/path arg1 arg2 arg3",
			args:           []string{"arg1", "arg2", "arg3"},
			envVars:        []string{"KEY1=VALUE1", "KEY2=VALUE2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(tt.command, tt.args...)
			cmd.Env = tt.envVars

			actualOutput := buildCommandDebugString(cmd)
			assert.Equal(t, tt.expectedOutput, actualOutput)
		})
	}
}
