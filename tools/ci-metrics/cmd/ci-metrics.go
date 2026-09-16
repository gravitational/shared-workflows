// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/cmd/cmds"
	cli "github.com/urfave/cli/v3"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run builds the root command and executes it.
func run(ctx context.Context, args []string) error {
	var cancel context.CancelFunc
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	cmd := &cli.Command{
		Name:    "ci-metrics",
		Usage:   "Compaction and reporting over normalized CI test results",
		Suggest: true,
		Commands: []*cli.Command{
			cmds.NewMigrateCommand(),
		},
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Name:  "timeout",
				Local: false,
				Usage: "Maximum execution time (e.g. 30s, 2m); 0 means no timeout",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if timeout := cmd.Duration("timeout"); timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, timeout)
			}
			return ctx, nil
		},
	}

	return cmd.Run(ctx, args)
}
