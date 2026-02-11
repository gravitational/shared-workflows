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
	"errors"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	cfg := Config{
		MaxRetries: 3,
		Initial:    time.Millisecond,
		Executor: func(ctx context.Context, name string, args ...string) error {
			return nil
		},
	}

	if err := Run(context.Background(), cfg, "echo", "hello"); err != nil {
		t.Errorf("Run() failed unexpectedly: %v", err)
	}
}

func TestRun_Fail(t *testing.T) {
	cfg := Config{
		MaxRetries: 2,
		Initial:    time.Millisecond,
		Executor: func(ctx context.Context, name string, args ...string) error {
			return errors.New("simulated failure")
		},
	}

	err := Run(context.Background(), cfg, "fail_cmd")
	if err == nil {
		t.Error("Run() expected error, got nil")
	}
}

func TestRun_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{
		MaxRetries: 5,
		Initial:    time.Second,
		Executor: func(ctx context.Context, name string, args ...string) error {
			return errors.New("fail")
		},
	}

	err := Run(ctx, cfg, "slow_cmd")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() expected context.Canceled, got %v", err)
	}
}

func TestBackoff(t *testing.T) {
	tests := []struct {
		name      string
		initial   time.Duration
		attempt   int
		resultMin time.Duration
		resultMax time.Duration
	}{
		{
			name:      "Attempt 1 (1x initial)",
			initial:   1 * time.Second,
			attempt:   1,
			resultMin: 1 * time.Second,
			resultMax: 2 * time.Second,
		},
		{
			name:      "Attempt 2 (2x initial)",
			initial:   1 * time.Second,
			attempt:   2,
			resultMin: 2 * time.Second,
			resultMax: 4 * time.Second,
		},
		{
			name:      "Zero Initial",
			initial:   0,
			attempt:   1,
			resultMin: 0,
			resultMax: 0,
		},
		{
			name:      "Negative Attempt",
			initial:   time.Second,
			attempt:   -1,
			resultMin: time.Second,
			resultMax: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := backoff(tt.initial, tt.attempt)

			if delay < tt.resultMin || delay > tt.resultMax {
				t.Errorf("backoff() = %v, want [%v, %v)", delay, tt.resultMin, tt.resultMax)
			}
		})
	}
}
