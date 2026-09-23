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

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/cmd/cmds"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run builds the root command and executes it.
func run(ctx context.Context, args []string) error {
	app := kingpin.New("ci-metrics", "Compaction and reporting over normalized CI test results")
	app.HelpFlag.Short('h')

	timeout := app.Flag(
		"timeout",
		"Maximum execution time (e.g. 30s, 2m); 0 means no timeout",
	).Default("0").Duration()

	migrateCmd := cmds.NewMigrateCommand(app)
	reportCmd := cmds.NewReportCommand(app)

	command, err := app.Parse(args)
	if err != nil {
		return trace.Wrap(err, "parsing command line arguments")
	}

	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	switch command {
	case migrateCmd.FullCommand():
		return trace.Wrap(migrateCmd.Run(ctx))
	case reportCmd.FullCommand():
		return trace.Wrap(reportCmd.Run(ctx))
	default:
		return trace.NotImplemented("unimplemented command %q", command)
	}
}
