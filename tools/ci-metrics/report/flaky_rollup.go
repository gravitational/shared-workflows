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
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

// FlakyRollupName is the registered name of the window rollup report.
const FlakyRollupName = "flaky_rollup"

const flakyRollupQuery = "rollup"

func init() {
	Register(Definition{
		Name:      FlakyRollupName,
		Summary:   "Rank the flakiest tests over the window as a whole",
		NewParams: func() any { return &FlakyParams{} },
		Queries:   flakyRollupQueries,
		Render:    flakyRollupRender,
	})
}

// flakyRollupQueries renders the statement ranking tests over the window.
func flakyRollupQueries(scope Scope, params any) (map[string]Statement, error) {
	return flakyStatement(scope, params, flakyRollupQuery, "flaky_rollup.sql")
}

// flakyRollupRender builds a single ranked table for the window.
func flakyRollupRender(scope Scope, params any, results map[string]*athena.Result) (*Document, error) {
	p, err := flakyParams(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rs, err := flakyResult(results, flakyRollupQuery, FlakyRollupName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rows, err := decodeFlakyRollupRows(rs)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	doc := &Document{
		ID:       FlakyRollupName,
		Title:    "Top flaky tests | window rollup",
		Subtitle: scope.Database + "." + scope.Tables.Testcases + " | " + Window(scope.From, scope.To),
	}

	if len(rows) == 0 {
		doc.Headline = "No flaky tests found in the window."
		doc.Sections = []Section{{
			Heading: "Summary",
			Notes: append([]string{
				"No test both passed and failed over the window's totals with at " +
					"least " + Int(int64(p.MinExecs)) + " executions.",
			}, flakyScoreNotes(p, "over the window")...),
		}}
		return doc, nil
	}

	// Ordered by score, so the first row is the flakiest test.
	flakiest := rows[0].TestName
	distinct := distinctFlakyTests(rows)

	doc.Headline = Int(int64(distinct)) + " flaky test(s) over the window; flakiest " + flakiest

	doc.Sections = append(doc.Sections, Section{
		Heading: "Summary",
		Metrics: []Metric{
			{Name: "Flaky tests listed", Value: Int(int64(distinct))},
			{Name: "Flakiest test", Value: flakiest},
		},
		Notes: append(flakyScoreNotes(p, "over the window"),
			"Scores are the window's totals, not a sum or average of the daily "+
				"rows the "+FlakyDailyName+" report produces: a test that fails "+
				"every run one day and passes every run the next is excluded from "+
				"both of those days but can still appear here."),
	})

	section := Section{
		Heading: "Window rollup | " + Window(scope.From, scope.To),
		Table:   flakyTable(rows),
		Notes:   flakyCapNote(rows, p.Top, ""),
	}
	doc.Sections = append(doc.Sections, section)

	return doc, nil
}

// decodeFlakyRollupRows reads the result into typed rows. There is no day
// column, so [flakyRow.Day] stays empty.
func decodeFlakyRollupRows(rs *athena.Result) ([]flakyRow, error) {
	return collectFlakyRows(flakyScanner(rs), rs)
}
