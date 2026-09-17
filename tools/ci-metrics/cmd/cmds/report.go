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
	"slices"
	"time"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/reporter"
)

// ReportCommand runs statistical reports over the normalized test records and
// writes them to the configured destinations.
//
// The orchestration lives here rather than in the report package because this
// is the composition root: reporter imports report for the document type, so
// report cannot import reporter back without a cycle.
type ReportCommand struct {
	cmd *kingpin.CmdClause

	athenaConfig athena.Config
	reportConfig *report.Config

	configPath string
	dryRun     bool

	from time.Time
	to   time.Time
	days int

	// fromSet and daysSet record whether the user passed the flag, since
	// kingpin does not support mutually exclusive flag groups.
	fromSet bool
	daysSet bool
}

// NewReportCommand registers the report subcommand on app.
func NewReportCommand(app *kingpin.Application) *ReportCommand {
	c := &ReportCommand{
		to: time.Now().UTC(),
	}

	c.cmd = app.Command("report", "Run statistical reports over normalized CI test results")

	// Deliberately not Required: [report.LoadConfig] also accepts the config
	// body itself in EnvConfigBody, and only sees that fallback when the path
	// is empty. With no source at all it falls back to [report.DefaultConfig].
	c.cmd.Flag("config", "YAML configuration for reporters, or '-' to read stdin").
		Short('c').
		PlaceHolder("PATH").
		Envar(report.EnvConfigPath).
		StringVar(&c.configPath)

	c.cmd.Flag("dryrun", "Print the statements without executing them").
		BoolVar(&c.dryRun)

	// --from and --days are mutually exclusive; the condition is enforced in
	// window, which is also where the config's own window is applied.
	c.cmd.Flag("from", "First day to report on, inclusive (YYYY-MM-DD); mutually exclusive with --days").
		PlaceHolder("YYYY-MM-DD").
		IsSetByUser(&c.fromSet).
		SetValue(&dateValue{target: &c.from})

	c.cmd.Flag("to", "Last day to report on, inclusive (YYYY-MM-DD); defaults to today").
		PlaceHolder("YYYY-MM-DD").
		SetValue(&dateValue{target: &c.to})

	c.cmd.Flag("days", "Report on the last N days, inclusive of --to; mutually exclusive with --from").
		PlaceHolder("N").
		IsSetByUser(&c.daysSet).
		IntVar(&c.days)

	registerAthenaConfigFlags(c.cmd, &c.athenaConfig)

	return c
}

// FullCommand returns the command path used to dispatch on the parse result.
func (c *ReportCommand) FullCommand() string {
	return c.cmd.FullCommand()
}

// Run loads the config and executes the selected reports.
func (c *ReportCommand) Run(ctx context.Context) error {
	var err error
	if c.reportConfig, err = report.LoadConfig(c.configPath); err != nil {
		return trace.Wrap(err, "loading config")
	}

	return trace.Wrap(c.run(ctx))
}

func (c *ReportCommand) run(ctx context.Context) error {
	from, to, err := c.window()
	if err != nil {
		return trace.Wrap(err)
	}

	scope := report.Scope{
		Database: c.athenaConfig.Database,
		Tables:   c.reportConfig.Tables,
		From:     from,
		To:       to,
	}

	sinks, byName, err := c.buildReporters()
	if err != nil {
		return trace.Wrap(err)
	}
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

	fmt.Printf("%s, %s .. %s, reporters: %s\n",
		c.athenaConfig.Database, report.Day(from), report.Day(to), sinks.Name())

	var errs []error
	for _, rc := range c.reportConfig.Reports {
		def, ok := report.Get(rc.Name)
		if !ok {
			errs = append(errs, trace.BadParameter("unknown report %q", rc.Name))
			continue
		}

		params, err := rc.DecodeParams(def)
		if err != nil {
			errs = append(errs, trace.Wrap(err))
			continue
		}

		doc, err := report.Execute(ctx, exec, scope, rc.Name, params)
		if err != nil {
			errs = append(errs, trace.Wrap(err, "running report %s", rc.Name))
			continue
		}

		// Dry runs don't print empty reports, we may wish ot change this for manual testing.
		if c.dryRun {
			continue
		}

		target := sinks
		// A nil index means --reporter replaced the configured destinations,
		// so every report goes to the same overridden set.
		if byName != nil {
			target, err = selectSinks(byName, rc.Reporters)
			if err != nil {
				errs = append(errs, trace.Wrap(err, "report %s", rc.Name))
				continue
			}
		}

		if err := target.Report(ctx, doc); err != nil {
			errs = append(errs, trace.Wrap(err, "reporting %s", rc.Name))
		}
	}

	return trace.NewAggregate(errs...)
}

// window resolves the reporting period from flags, falling back to the
// configured number of days.
func (c *ReportCommand) window() (from, to time.Time, err error) {
	to = c.to.UTC()

	if c.fromSet && c.daysSet {
		return from, to, trace.BadParameter("--from and --days are mutually exclusive")
	}

	days := c.reportConfig.Window.Days
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

// buildReporters constructs the destinations the selected reports will use.
//
// It returns every reporter as a [reporter.Multi] for preflight and close, and
// a per-name index so each report can be sent only to its own destinations.
func (c *ReportCommand) buildReporters() (reporter.Multi, map[string]reporter.Reporter, error) {
	var names []string
	for _, rc := range c.reportConfig.Reports {
		for _, name := range rc.Reporters {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	// Sorted so the constructed order, and any error, is deterministic.
	slices.Sort(names)

	byName := make(map[string]reporter.Reporter, len(names))
	var all reporter.Multi
	for _, name := range names {
		cfg, ok := c.reportConfig.Reporters[name]
		if !ok {
			return nil, nil, trace.BadParameter("undefined reporter %q", name)
		}

		r, err := reporter.New(name, cfg, os.Stdout)
		if err != nil {
			return nil, nil, trace.Wrap(err)
		}
		byName[name] = r
		all = append(all, r)
	}

	return all, byName, nil
}

// selectSinks picks the named reporters out of the index.
func selectSinks(byName map[string]reporter.Reporter, names []string) (reporter.Multi, error) {
	out := make(reporter.Multi, 0, len(names))
	for _, name := range names {
		r, ok := byName[name]
		if !ok {
			return nil, trace.BadParameter("undefined reporter %q", name)
		}
		out = append(out, r)
	}
	return out, nil
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
