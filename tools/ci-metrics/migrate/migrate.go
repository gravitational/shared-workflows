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

package migrate

import (
	"context"
	"embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
	"github.com/gravitational/shared-workflows/tools/ci-metrics/partition"
)

//go:embed sql/*.sql
var sqlFS embed.FS

var templates = template.Must(
	template.New("migrate").
		Option("missingkey=error").
		ParseFS(sqlFS, "sql/*.sql"),
)

// identifierPattern matches a bare SQL identifier.
var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Types returns the record types with an embedded migration template.
func Types() []string {
	var out []string
	for _, t := range templates.Templates() {
		if name, ok := strings.CutSuffix(t.Name(), ".sql"); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// TableMigration is one migration
type TableMigration struct {
	// Type is one of [Types].
	Type string
	// Source is the JSONL table, partitioned by (repository, year, month, day).
	Source string
	// Destination is the Parquet table, partitioned by (repository, dt).
	Destination string
}

func (t TableMigration) checkAndSetDefaults() error {
	if templates.Lookup(t.Type+".sql") == nil {
		return trace.BadParameter(
			"unknown type %q, want one of %s", t.Type, strings.Join(Types(), ", "))
	}
	if !identifierPattern.MatchString(t.Source) {
		return trace.BadParameter("invalid source table %q", t.Source)
	}
	if !identifierPattern.MatchString(t.Destination) {
		return trace.BadParameter("invalid destination table %q", t.Destination)
	}
	if t.Source == t.Destination {
		return trace.BadParameter("source and destination are both %q", t.Source)
	}
	return nil
}

// Options configures a migration run.
type Options struct {
	Database       string
	TableMigration TableMigration
	From, To       time.Time
}

func (o *Options) checkAndSetDefaults() error {
	if !identifierPattern.MatchString(o.Database) {
		return trace.BadParameter("invalid database name %q", o.Database)
	}
	if err := o.TableMigration.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	if o.From.After(o.To) {
		return trace.BadParameter("invalid time period %q -> %q",
			o.From.Format(time.DateOnly), o.To.Format(time.DateOnly))
	}

	return nil
}

// render produces the statement migrating one day.
// Assumes parameters have already been validated by [Run]
func render(database string, t TableMigration, day time.Time) (string, error) {
	predicate, err := partition.PeriodPredicate("s", day, day)
	if err != nil {
		return "", trace.Wrap(err)
	}

	var buf strings.Builder
	err = templates.ExecuteTemplate(&buf, t.Type+".sql", struct {
		Database        string
		SourceTable     string
		DestTable       string
		SourcePredicate string
		Day             string
	}{
		Database:        database,
		SourceTable:     t.Source,
		DestTable:       t.Destination,
		SourcePredicate: predicate,
		Day:             day.Format(time.DateOnly),
	})
	if err != nil {
		return "", trace.Wrap(err, "rendering %s template", t.Type)
	}

	return strings.TrimRight(buf.String(), "\n"), nil
}

// Run migrates each day in the period
func Run(ctx context.Context, exec athena.Executor, opts Options) error {
	if err := opts.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	started := time.Now()
	var statements int
	var bytesScanned int64

	for day := opts.From; !day.After(opts.To); day = day.AddDate(0, 0, 1) {
		statement, err := render(opts.Database, opts.TableMigration, day)
		if err != nil {
			return trace.Wrap(err, "rendering statement")
		}

		result, err := exec.Execute(ctx, statement)
		if err != nil {
			return trace.Wrap(err, "migrating %s", day.Format(time.DateOnly))
		}

		statements++
		bytesScanned += result.DataScannedBytes
		fmt.Printf("  %s  %9s scanned  %6s  %s\n",
			day.Format(time.DateOnly),
			humanize.IBytes(uint64(result.DataScannedBytes)),
			result.EngineTime.Round(100*time.Millisecond),
			result.QueryExecutionID,
		)
	}

	fmt.Printf("%d statement(s), %s scanned, %s elapsed\n",
		statements, humanize.IBytes(uint64(bytesScanned)), time.Since(started).Round(time.Second))
	return nil
}
