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
	"strings"
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

// MigrateCommand copies one table of JSONL test records into its Parquet
// counterpart over one period.
type MigrateCommand struct {
	cmd *kingpin.CmdClause

	athenaConfig athena.Config

	table migrate.TableMigration
	from  time.Time
	to    time.Time
	days  int

	// fromSet and daysSet record whether the user passed the flag, since
	// kingpin does not support mutually exclusive flag groups.
	fromSet bool
	daysSet bool

	dryRun bool
}

// registerAthenaConfigFlags creates the CLI interface to configure [athena.Config].
func registerAthenaConfigFlags(cmd *kingpin.CmdClause, cfg *athena.Config) {
	cmd.Flag("database", "Database holding both tables").
		Required().
		StringVar(&cfg.Database)

	cmd.Flag("workgroup", "Athena workgroup").
		Default("primary").
		StringVar(&cfg.Workgroup)

	cmd.Flag("region", "AWS region; defaults to the ambient credential chain").
		StringVar(&cfg.Region)

	cmd.Flag("results", "S3 prefix for Athena query results; unset defers to the workgroup setting").
		StringVar(&cfg.OutputLocation)
}

// NewMigrateCommand registers the migrate subcommand on app.
func NewMigrateCommand(app *kingpin.Application) *MigrateCommand {
	c := &MigrateCommand{
		to: time.Now().UTC(),
	}

	c.cmd = app.Command("migrate", "Copy one table of JSONL test records into Parquet, one day at a time")

	c.cmd.Arg("type", "Record type, one of "+strings.Join(migrate.Types(), ", ")).
		Required().
		EnumVar(&c.table.Type, migrate.Types()...)

	c.cmd.Flag("source", "JSONL table to read").
		Required().
		StringVar(&c.table.Source)

	c.cmd.Flag("destination", "Parquet table to write").
		Required().
		StringVar(&c.table.Destination)

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
		from = to.AddDate(0, 0, -c.days)
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

	fmt.Printf("%s.%s -> %s.%s, %s .. %s\n",
		c.athenaConfig.Database, c.table.Source, c.athenaConfig.Database, c.table.Destination,
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
		Database:       c.athenaConfig.Database,
		TableMigration: c.table,
		From:           from,
		To:             to,
	}), "running migration")
}
