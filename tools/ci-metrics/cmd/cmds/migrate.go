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
	"slices"
	"strings"
	"time"

	"github.com/gravitational/trace"
	cli "github.com/urfave/cli/v3"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/migrate"
)

// migrateCommand copies one table of JSONL test records into its Parquet
// counterpart over one period.
type migrateCommand struct {
	athenaConfig athena.Config

	table migrate.TableMigration
	from  time.Time
	to    time.Time
	days  int

	dryRun bool
}

// flagsForAthenaConfig creates CLI interface to configure [athena.Config]
func flagsForAthenaConfig(cfg *athena.Config) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "database",
			Aliases:     []string{"db"},
			Usage:       "Database holding both tables",
			Required:    true,
			Destination: &cfg.Database,
		},
		&cli.StringFlag{
			Name:        "workgroup",
			Usage:       "Athena workgroup",
			Value:       "primary",
			Destination: &cfg.Workgroup,
		},
		&cli.StringFlag{
			Name:        "region",
			Usage:       "AWS region; defaults to the ambient credential chain",
			Destination: &cfg.Region,
		},
		&cli.StringFlag{
			Name:        "output-location",
			Aliases:     []string{"results"},
			Usage:       "S3 prefix for Athena query results; unset defers to the workgroup setting",
			Destination: &cfg.OutputLocation,
		},
	}
}

// newMigrateCommand builds the migrate subcommand.
func NewMigrateCommand() *cli.Command {
	c := &migrateCommand{
		to: time.Now(),
	}

	return &cli.Command{
		Name:  "migrate",
		Usage: "Copy one table of JSONL test records into Parquet, one day at a time",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "type",
				Required:    true,
				Destination: &c.table.Type,
			},
		},
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"src"},
				Usage:       "JSONL table to read",
				Required:    true,
				Destination: &c.table.Source,
			},
			&cli.StringFlag{
				Name:        "destination",
				Aliases:     []string{"dst"},
				Usage:       "Parquet table to write",
				Required:    true,
				Destination: &c.table.Destination,
			},

			&cli.TimestampFlag{
				Name:  "to",
				Usage: "Last day to migrate, inclusive (YYYY-MM-DD)",
				Config: cli.TimestampConfig{
					Layouts:  []string{time.DateOnly},
					Timezone: time.UTC,
				},
				Destination: &c.to,
				Value:       time.Now().UTC(),
				DefaultText: "now",
			},
			&cli.BoolFlag{
				Name:        "dryrun",
				Aliases:     []string{"dry"},
				Usage:       "Print the statements without executing them",
				Destination: &c.dryRun,
			},
		},
			flagsForAthenaConfig(&c.athenaConfig)...),

		MutuallyExclusiveFlags: []cli.MutuallyExclusiveFlags{
			{
				Required: true,
				Flags: [][]cli.Flag{
					{
						&cli.TimestampFlag{
							Name:  "from",
							Usage: "First day to migrate (YYYY-MM-DD)",
							Config: cli.TimestampConfig{
								Layouts:  []string{time.DateOnly},
								Timezone: time.UTC,
							},
							Destination: &c.from,
						},
					},
					{
						&cli.IntFlag{
							Name:        "days",
							Usage:       "Migrate the last `N` days ending today",
							Destination: &c.days,
						},
					},
				},
			},
		},

		Action: func(ctx context.Context, cmd *cli.Command) error {
			return trace.Wrap(c.run(ctx, cmd))
		},
	}
}

func (c *migrateCommand) run(ctx context.Context, cmd *cli.Command) error {
	// The record type selects the column list, so an unknown one has no
	// sensible interpretation. Positional arguments carry no enum constraint,
	// so check it here.
	if !slices.Contains(migrate.Types(), c.table.Type) {
		return trace.BadParameter("unknown record type %q, expected one of %s",
			c.table.Type, strings.Join(migrate.Types(), ", "))
	}

	var from time.Time
	to := c.to
	if cmd.IsSet("days") {
		from = to.AddDate(0, 0, -c.days)
	} else {
		from = c.from.UTC()
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
