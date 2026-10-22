// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmds

import (
	"context"
	"fmt"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/migrate"
)

// dateValue parses a YYYY-MM-DD command line value into a UTC time. kingpin
// carries no time parser, so the flag supplies its own [kingpin.Value].
type dateValue struct {
	target *time.Time
}

// Set implements [kingpin.Value].
func (d *dateValue) Set(value string) error {
	parsed, err := time.ParseInLocation(time.DateOnly, value, time.UTC)
	if err != nil {
		return trace.BadParameter("invalid date %q, expected YYYY-MM-DD", value)
	}
	*d.target = parsed
	return nil
}

// String implements [kingpin.Value].
func (d *dateValue) String() string {
	if d.target == nil || d.target.IsZero() {
		return ""
	}
	return d.target.Format(time.DateOnly)
}

// MigrateCommand copies all JSONL test record tables into their Parquet
// counterparts over one period.
type MigrateCommand struct {
	cmd *kingpin.CmdClause

	athenaConfig athena.Config

	from time.Time
	to   time.Time
	days int

	// fromSet and daysSet record whether the user passed the flag, since
	// kingpin does not support mutually exclusive flag groups.
	fromSet bool
	daysSet bool

	dryRun bool
}

// NewMigrateCommand registers the migrate subcommand on app.
func NewMigrateCommand(app *kingpin.Application) *MigrateCommand {
	c := &MigrateCommand{
		to: time.Now().UTC(),
	}

	c.cmd = app.Command("migrate", "Copy all JSONL test record tables into Parquet, one day at a time")

	// --from and --days are mutually exclusive, and exactly one is required.
	// Both conditions are enforced in Run.
	c.cmd.Flag("from", "First day to migrate (YYYY-MM-DD); mutually exclusive with --days").
		PlaceHolder("YYYY-MM-DD").
		IsSetByUser(&c.fromSet).
		SetValue(&dateValue{target: &c.from})

	c.cmd.Flag("days", "Migrate the last N days ending today; mutually exclusive with --from").
		PlaceHolder("N").
		IsSetByUser(&c.daysSet).
		IntVar(&c.days)

	c.cmd.Flag("to", "Last day to migrate, inclusive (YYYY-MM-DD); defaults to today").
		PlaceHolder("YYYY-MM-DD").
		SetValue(&dateValue{target: &c.to})

	c.cmd.Flag("dryrun", "Print the statements without executing them").
		BoolVar(&c.dryRun)

	registerAthenaConfigFlags(c.cmd, &c.athenaConfig)

	return c
}

// FullCommand returns the command path used to dispatch on the parse result.
func (c *MigrateCommand) FullCommand() string {
	return c.cmd.FullCommand()
}

func (c *MigrateCommand) Run(ctx context.Context) error {
	var from time.Time
	to := c.to

	switch {
	case c.fromSet && c.daysSet:
		return trace.BadParameter("--from and --days are mutually exclusive")
	case c.daysSet:
		if c.days < 1 {
			return trace.BadParameter("days must be at least 1, got %d", c.days)
		}
		// Inclusive of --to, so --days 1 migrates a single day.
		from = to.AddDate(0, 0, -(c.days - 1))
	case c.fromSet:
		from = c.from.UTC()
	default:
		return trace.BadParameter("one of --from or --days is required")
	}

	if to.Before(from) {
		return trace.BadParameter(
			"period start %s is after period end %s",
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}

	fmt.Printf("migrating %s .. %s\n",
		from.Format(time.DateOnly), to.Format(time.DateOnly))

	var exec athena.Executor
	var err error

	if !c.dryRun {
		if exec, err = athena.NewFromConfig(ctx, c.athenaConfig); err != nil {
			return trace.Wrap(err, "creating athena client")
		}
	} else {
		if exec, err = athena.NewNoop(ctx, c.athenaConfig); err != nil {
			return trace.Wrap(err, "creating no-op client")
		}
	}

	return trace.Wrap(migrate.Run(ctx, exec, migrate.Options{
		Database: c.athenaConfig.Database,
		From:     from,
		To:       to,
	}), "running migration")
}
