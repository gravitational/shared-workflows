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
	"io"
	"os"
	"slices"
	"strings"
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

	only      []string
	reporters []string

	// out is where reports and the run header are written. A nil out means
	// [os.Stdout]; tests set it to capture the output.
	out io.Writer
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

	c.cmd.Flag("only", "Run only the named report(s), repeatable").
		PlaceHolder("REPORT").
		StringsVar(&c.only)

	c.cmd.Flag("reporter", "Override every configured destination with the given reporter "+
		"type(s), repeatable; use this to iterate locally without posting anywhere").
		PlaceHolder("TYPE").
		StringsVar(&c.reporters)

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

// writer resolves where to send report output, falling back to stdout when the
// command carries no writer of its own.
func (c *ReportCommand) writer() io.Writer {
	if c.out != nil {
		return c.out
	}
	return os.Stdout
}

func (c *ReportCommand) run(ctx context.Context) error {
	out := c.writer()

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

	selected, err := c.selectReports()
	if err != nil {
		return trace.Wrap(err)
	}

	sinks, byName, err := c.buildReporters(out, selected)
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

	if _, err := fmt.Fprintf(out, "%s, %s .. %s, reporters: %s\n",
		c.athenaConfig.Database, report.Day(from), report.Day(to), sinks.Name()); err != nil {
		return trace.Wrap(err, "writing header")
	}

	// One failing report should not hide the others, so collect and continue.
	var errs []error
	for _, rc := range selected {
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

		// A dry run has printed the statements and has no rows, so rendering
		// would only produce a document that looks like a real empty result.
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

// selectReports applies --only to the configured reports.
func (c *ReportCommand) selectReports() ([]report.ReportConfig, error) {
	if len(c.only) == 0 {
		return c.reportConfig.Reports, nil
	}

	var out []report.ReportConfig
	seen := make(map[string]struct{}, len(c.only))
	for _, name := range c.only {
		i := slices.IndexFunc(c.reportConfig.Reports, func(r report.ReportConfig) bool {
			return r.Name == name
		})
		if i < 0 {
			return nil, trace.BadParameter(
				"--only %s is not a configured report; configured: %s",
				name, configuredNames(c.reportConfig.Reports))
		}
		// Repeating --only must not deliver the same document twice.
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, c.reportConfig.Reports[i])
	}

	return out, nil
}

// buildReporters constructs the destinations the selected reports will use.
//
// It returns every reporter as a [reporter.Multi] for preflight and close, and
// a per-name index so each report can be sent only to its own destinations.
// When --reporter overrides the config there are no names to index, so the
// index is nil and every report goes to the same set.
//
// Only the reporters the selected reports actually name are built, so a
// configured destination that nothing in this run refers to cannot fail it.
func (c *ReportCommand) buildReporters(
	out io.Writer, selected []report.ReportConfig,
) (reporter.Multi, map[string]reporter.Reporter, error) {
	if len(c.reporters) > 0 {
		var all reporter.Multi
		for _, typ := range c.reporters {
			r, err := reporter.New(typ, report.ReporterConfig{Type: typ}, out)
			if err != nil {
				return nil, nil, trace.Wrap(err, "--reporter %s", typ)
			}
			all = append(all, r)
		}
		return all, nil, nil
	}

	var names []string
	for _, rc := range selected {
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

		r, err := reporter.New(name, cfg, out)
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
func (c *ReportCommand) executor(ctx context.Context) (athena.Executor, error) {
	if c.dryRun {
		q, err := athena.NewNoop(ctx, c.athenaConfig)
		if err != nil {
			return nil, trace.Wrap(err, "creating no-op client")
		}
		return q, nil
	}

	q, err := athena.NewFromConfig(ctx, c.athenaConfig)
	if err != nil {
		return nil, trace.Wrap(err, "creating athena client")
	}
	return q, nil
}

// configuredNames renders the configured report names for an error message.
func configuredNames(reports []report.ReportConfig) string {
	if len(reports) == 0 {
		return "(none)"
	}

	names := make([]string, 0, len(reports))
	for _, r := range reports {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}
