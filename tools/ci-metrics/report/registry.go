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

package report

import (
	"context"
	"regexp"
	"slices"
	"time"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

// identifierPattern matches a bare SQL identifier. Report SQL interpolates
// identifiers textually, so every one must be checked against this before it
// reaches a statement.
var identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Tables names the tables a report reads.
type Tables struct {
	// Meta is the JSONL meta table, partitioned by (repository, year, month, day).
	Meta string `yaml:"meta"`
	// Testcases is the JSONL testcase table, partitioned the same way.
	Testcases string `yaml:"testcases"`
}

func (t *Tables) checkAndSetDefaults() error {
	if t.Meta == "" {
		t.Meta = "meta_v2_parquet"
	}
	if t.Testcases == "" {
		t.Testcases = "testcases_v2_parquet"
	}
	if !identifierPattern.MatchString(t.Meta) {
		return trace.BadParameter("invalid meta table %q", t.Meta)
	}
	if !identifierPattern.MatchString(t.Testcases) {
		return trace.BadParameter("invalid testcases table %q", t.Testcases)
	}
	return nil
}

// Scope is the context every report runs in: where to read from and over what
// period.
type Scope struct {
	Database string
	Tables   Tables
	// From and To bound the window, inclusive, and are expected in UTC.
	From, To time.Time
}

func (s *Scope) checkAndSetDefaults() error {
	if !identifierPattern.MatchString(s.Database) {
		return trace.BadParameter("invalid database name %q", s.Database)
	}
	if err := s.Tables.checkAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	if s.From.After(s.To) {
		return trace.BadParameter("invalid time period %q -> %q",
			Day(s.From), Day(s.To))
	}
	return nil
}

// Statement is one rendered SQL statement belonging to a report.
type Statement struct {
	SQL string
}

// Definition is everything the runner needs to know about one report.
//
// Queries is kept apart from Render so that each half is testable on its own:
// the statements can be asserted against a golden file with no AWS account,
// and the document can be built from a fixture result set with no SQL.
type Definition struct {
	// Name identifies the report on the command line and in config.
	Name string
	// Summary is a one-line description of what the report shows.
	Summary string
	// NewParams returns a zero value of this report's parameter type, used as
	// the decode target for a config `params:` block.
	NewParams func() any
	// Queries renders the statements the report needs, keyed by a name the
	// report chooses. The map allows a report to run more than one query, as a
	// window-over-window comparison must.
	Queries func(Scope, any) (map[string]Statement, error)
	// Render turns the results of Queries into a document.
	Render func(Scope, any, map[string]*athena.Result) (*Document, error)
}

// registry holds the known reports. It is written only by [Register] from
// package init and read-only afterwards, which keeps it safe under the
// repeated parallel test runs that `make test` performs.
var registry = map[string]Definition{}

// Register adds a report to the registry. It panics on a malformed or
// duplicate definition because both are programming errors that should fail
// the build's first test rather than a production run.
func Register(d Definition) {
	switch {
	case d.Name == "":
		panic("report: Register with an empty name")
	case d.NewParams == nil:
		panic("report: Register " + d.Name + " without NewParams")
	case d.Queries == nil:
		panic("report: Register " + d.Name + " without Queries")
	case d.Render == nil:
		panic("report: Register " + d.Name + " without Render")
	}

	if _, dup := registry[d.Name]; dup {
		panic("report: duplicate registration of " + d.Name)
	}
	registry[d.Name] = d
}

// Get returns the named report.
func Get(name string) (Definition, bool) {
	d, ok := registry[name]
	return d, ok
}

// Names returns the registered report names, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// Execute runs one report end to end and returns its document. It takes an
// [athena.Executor] rather than a concrete client so tests can supply a mock,
// and it knows nothing about reporters, which keeps the dependency between the
// two packages pointing one way.
func Execute(ctx context.Context, q athena.Executor, scope Scope, name string, params any) (*Document, error) {
	def, ok := Get(name)
	if !ok {
		return nil, trace.BadParameter("unknown report %q", name)
	}
	if err := scope.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "invalid scope configuration")
	}

	if params == nil {
		params = def.NewParams()
	}

	statements, err := def.Queries(scope, params)
	if err != nil {
		return nil, trace.Wrap(err, "building %s queries", name)
	}

	results := make(map[string]*athena.Result, len(statements))
	for key, stmt := range statements {
		rs, err := q.Execute(ctx, stmt.SQL)
		if err != nil {
			return nil, trace.Wrap(err, "running %s query %s", name, key)
		}
		results[key] = rs
	}

	doc, err := def.Render(scope, params, results)
	if err != nil {
		return nil, trace.Wrap(err, "rendering %s", name)
	}

	doc.Meta.From, doc.Meta.To = scope.From, scope.To
	doc.Meta.GeneratedAt = time.Now().UTC()
	for _, rs := range results {
		if rs == nil {
			continue
		}
		doc.Meta.DataScannedBytes += rs.DataScannedBytes
		// A dry run reports no execution, so there is no ID to record.
		if rs.QueryExecutionID != "" {
			doc.Meta.QueryExecutionIDs = append(doc.Meta.QueryExecutionIDs, rs.QueryExecutionID)
		}
	}
	return doc, nil
}
