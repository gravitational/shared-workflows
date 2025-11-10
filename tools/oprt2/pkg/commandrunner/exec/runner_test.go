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
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
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
		existingHooks []commandrunner.Hook
		providedHooks []commandrunner.Hook
		expectedHooks []commandrunner.Hook
	}{
		{
			name: "no hooks",
		},
		{
			name: "no new hooks",
			existingHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
			},
			expectedHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
			},
		},
		{
			name: "new hook",
			existingHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
			},
			providedHooks: []commandrunner.Hook{
				testHookA,
			},
			expectedHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
				testHookA,
			},
		},
		{
			name: "new hooks",
			existingHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
			},
			providedHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
				testHookA,
			},
			expectedHooks: []commandrunner.Hook{
				testHookA,
				testHookB,
				testHookA,
				testHookB,
				testHookA,
			},
		},
		{
			name: "nil hook",
			providedHooks: []commandrunner.Hook{
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

func TestRun(t *testing.T) {
	t.Run("basic no-op", func(t *testing.T) {
		assert.NoError(t, NewRunner().Run(t.Context(), "true"))
	})

	t.Run("multiple command hooks with one failing", func(t *testing.T) {
		ctx := t.Context()

		buildHook := func(name string, err error) *mockHook {
			h := &mockHook{}
			h.On("Name").Maybe().Return(name)
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
		h.On("Command", ctx, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			// Valiate the command parameters
			cmd, ok := args.Get(1).(*exec.Cmd)
			require.True(t, ok)
			require.NotNil(t, cmd)

			assert.Equal(t, "echo", filepath.Base(cmd.Path))
			assert.Equal(t, []string{"echo", "base arg"}, cmd.Args)

			shPath, err := exec.LookPath("sh")
			require.NoError(t, err)

			// Change the parameters
			cmd.Path = shPath
			cmd.Args = []string{"sh", "-c", "printf 'env var: %s' \"${TEST_VAR}\""}
			cmd.Env = append(cmd.Env, "TEST_VAR=test value")
			cmd.Stdout = &stdoutCapture
		}).Twice()

		h.On("Name").Return("Test hook")

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
