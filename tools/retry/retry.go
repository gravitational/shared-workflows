/*
Copyright 2026 Gravitational, Inc.

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

package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"time"
)

// Executor defines how a command is run.
type Executor func(ctx context.Context, name string, args ...string) error

// Config holds the configuration for the retry logic.
type Config struct {
	MaxRetries int
	Initial    time.Duration
	Executor   Executor
}

// Run executes the specified command with exponential backoff.
func Run(ctx context.Context, cfg Config, name string, args ...string) error {
	runCmd := cfg.Executor
	if runCmd == nil {
		runCmd = func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			return cmd.Run()
		}
	}

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		if attempt > 0 {
			delay := backoff(cfg.Initial, attempt)
			fmt.Fprintf(os.Stderr, "retrying in %v...\n", delay.Round(time.Second))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		if err := runCmd(ctx, name, args...); err != nil {
			continue
		}

		return nil
	}

	return fmt.Errorf("command failed after %d retries", cfg.MaxRetries)
}

// backoff calculates a random duration within the range [initial * 2^(attempt-1), initial * 2^attempt).
func backoff(initial time.Duration, attempt int) time.Duration {
	if initial <= 0 {
		return 0
	}

	if attempt <= 0 {
		return initial
	}

	base := initial * (1 << (attempt - 1))
	jitter := rand.Int63n(int64(base))

	return base + time.Duration(jitter)
}
