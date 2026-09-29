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
	"os"
	"strconv"
	"strings"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/reporter"
)

// defaultWindowDays is the window used when neither --from nor --days is set.
const defaultWindowDays = 14

// ReportCommand runs the flaky test report over the normalized test records
// and writes it to stdout.
//
// The orchestration lives here rather than in the report package because this
// is the composition root: reporter imports report for the document type, so
// report cannot import reporter back without a cycle.
type ReportCommand struct {
	cmd *kingpin.CmdClause

	athenaConfig athena.Config
	tables       report.Tables
	params       report.FlakyParams

	dryRun bool

	from      time.Time
	to        time.Time
	days      int
	yesterday bool

	// fromSet, toSet and daysSet record whether the user passed the flag,
	// since kingpin does not support mutually exclusive flag groups.
	fromSet bool
	toSet   bool
	daysSet bool
}

// NewReportCommand registers the report subcommand on app.
func NewReportCommand(app *kingpin.Application) *ReportCommand {
	c := &ReportCommand{
		to: time.Now().UTC(),
	}

	c.cmd = app.Command("report", "Rank the flakiest tests over a window of normalized CI test results")

	c.cmd.Flag("dryrun", "Print the statements without executing them").
		BoolVar(&c.dryRun)

	// --from and --days are mutually exclusive; the condition is enforced in
	// window.
	c.cmd.Flag("from", "First day to report on, inclusive (YYYY-MM-DD); mutually exclusive with --days").
		PlaceHolder("YYYY-MM-DD").
		IsSetByUser(&c.fromSet).
		SetValue(&dateValue{target: &c.from})

	c.cmd.Flag("to", "Last day to report on, inclusive (YYYY-MM-DD); defaults to today").
		PlaceHolder("YYYY-MM-DD").
		IsSetByUser(&c.toSet).
		SetValue(&dateValue{target: &c.to})

	c.cmd.Flag("days", "Report on the last N days, inclusive of --to; mutually exclusive with --from. "+
		"Defaults to "+strconv.Itoa(defaultWindowDays)+" when --from is unset").
		PlaceHolder("N").
		IsSetByUser(&c.daysSet).
		IntVar(&c.days)

	c.cmd.Flag("yesterday", "Report on yesterday (UTC) only; mutually exclusive with --from, --to and --days").
		BoolVar(&c.yesterday)

	c.cmd.Flag("branches", "Comma-separated full refs whose runs count, merge-queue runs targeting them included").
		Default(strings.Join(report.DefaultFlakyBranches, ",")).
		PlaceHolder("REF,...").
		SetValue(&csvValue{target: &c.params.Branches})

	c.cmd.Flag("top", "Number of tests to list").
		Default(strconv.Itoa(report.DefaultFlakyTop)).
		IntVar(&c.params.Top)

	c.cmd.Flag("min-execs", "Minimum executions over the window before a test is considered").
		Default(strconv.Itoa(report.DefaultFlakyMinExecs)).
		IntVar(&c.params.MinExecs)

	c.cmd.Flag("meta-table", "Parquet meta table to read").
		Default(report.DefaultMetaTable).
		StringVar(&c.tables.Meta)

	c.cmd.Flag("testcases-table", "Parquet testcases table to read").
		Default(report.DefaultTestcasesTable).
		StringVar(&c.tables.Testcases)

	registerAthenaConfigFlags(c.cmd, &c.athenaConfig)

	return c
}

// FullCommand returns the command path used to dispatch on the parse result.
func (c *ReportCommand) FullCommand() string {
	return c.cmd.FullCommand()
}

// Run executes the report.
func (c *ReportCommand) Run(ctx context.Context) error {
	from, to, err := c.window()
	if err != nil {
		return trace.Wrap(err)
	}

	sinks := reporter.Multi{reporter.NewStdout(reporter.TypeStdout, os.Stdout, 0)}
	defer func() {
		if err := sinks.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: closing reporters: %v\n", err)
		}
	}()

	// Athena charges by bytes scanned and a report can run for minutes, so
	// prove the destinations work before spending any of that.
	if err := sinks.Preflight(ctx); err != nil {
		return trace.Wrap(err, "reporter preflight")
	}

	exec, err := c.executor(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Printf("%s, %s, %s .. %s, branches: %s\n",
		report.FlakyName, c.athenaConfig.Database, report.Day(from), report.Day(to),
		strings.Join(c.params.Branches, ","))

	scope := report.Scope{
		Database: c.athenaConfig.Database,
		Tables:   c.tables,
		From:     from,
		To:       to,
	}

	doc, err := report.Execute(ctx, exec, scope, report.FlakyName, &c.params)
	if err != nil {
		return trace.Wrap(err, "running report %s", report.FlakyName)
	}

	// Dry runs don't print empty reports, we may wish to change this for manual testing.
	if c.dryRun {
		return nil
	}

	return trace.Wrap(sinks.Report(ctx, doc), "reporting %s", report.FlakyName)
}

// window resolves the reporting period from flags.
//
// Only the date part is used downstream, via [report.Day].
func (c *ReportCommand) window() (from, to time.Time, err error) {
	if c.yesterday {
		if c.fromSet || c.toSet || c.daysSet {
			return from, to, trace.BadParameter("--yesterday is mutually exclusive with --from, --to and --days")
		}
		// c.to defaults to now, so one day back is yesterday.
		day := c.to.UTC().AddDate(0, 0, -1)
		return day, day, nil
	}

	to = c.to.UTC()

	if c.fromSet && c.daysSet {
		return from, to, trace.BadParameter("--from and --days are mutually exclusive")
	}

	days := defaultWindowDays
	if c.daysSet {
		days = c.days
	}

	if c.fromSet {
		from = c.from.UTC()
	} else {
		if days < 1 {
			return from, to, trace.BadParameter("days must be at least 1, got %d", days)
		}
		// Inclusive of --to, so --days 1 reports on a single day.
		from = to.AddDate(0, 0, -(days - 1))
	}

	if to.Before(from) {
		return from, to, trace.BadParameter("period start %s is after period end %s",
			report.Day(from), report.Day(to))
	}

	return from, to, nil
}

// executor builds the Athena client, or its no-op stand-in under --dryrun.
func (c *ReportCommand) executor(ctx context.Context) (exe athena.Executor, err error) {
	if c.dryRun {
		exe, err = athena.NewNoop(ctx, c.athenaConfig)
		if err != nil {
			return nil, trace.Wrap(err, "creating no-op client")
		}
	} else {
		exe, err = athena.NewFromConfig(ctx, c.athenaConfig)
		if err != nil {
			return nil, trace.Wrap(err, "creating athena client")
		}
	}

	return
}

// csvValue parses a comma-separated command line value into a string slice.
// Each Set replaces the previous value, so an explicit flag overrides the
// default rather than appending to it.
type csvValue struct {
	target *[]string
}

func (v *csvValue) Set(value string) error {
	var out []string
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return trace.BadParameter("expected at least one value")
	}
	*v.target = out
	return nil
}

func (v *csvValue) String() string {
	if v.target == nil {
		return ""
	}
	return strings.Join(*v.target, ",")
}
