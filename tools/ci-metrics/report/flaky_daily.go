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

// FlakyDailyName is the registered name of the per-day flaky test report.
const FlakyDailyName = "flaky_daily"

const flakyDailyQuery = "daily"

func init() {
	Register(Definition{
		Name:      FlakyDailyName,
		Summary:   "Rank the flakiest tests within each day of the window",
		NewParams: func() any { return &FlakyParams{} },
		Queries:   flakyDailyQueries,
		Render:    flakyDailyRender,
	})
}

// flakyDailyQueries renders the statement ranking tests within each day.
func flakyDailyQueries(scope Scope, params any) (map[string]Statement, error) {
	return flakyStatement(scope, params, flakyDailyQuery, "flaky_daily.sql")
}

// flakyDailyRender builds one ranked table per day, most recent first.
func flakyDailyRender(scope Scope, params any, results map[string]*athena.Result) (*Document, error) {
	p, err := flakyParams(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rs, err := flakyResult(results, flakyDailyQuery, FlakyDailyName)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rows, err := decodeFlakyDailyRows(rs)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	doc := &Document{
		ID:       FlakyDailyName,
		Title:    "Top flaky tests | by day",
		Subtitle: scope.Database + "." + scope.Tables.Testcases + " | " + Window(scope.From, scope.To),
	}

	if len(rows) == 0 {
		doc.Headline = "No flaky tests found in the window."
		doc.Sections = []Section{{
			Heading: "Summary",
			Notes: append([]string{
				"No test both passed and failed within a single day with at least " +
					Int(int64(p.MinExecs)) + " executions.",
			}, flakyScoreNotes(p, "within a day")...),
		}}
		return doc, nil
	}

	// Ordered by day descending, so grouping in arrival order keeps the most
	// recent day first without re-sorting.
	var days []string
	byDay := map[string][]flakyRow{}
	for _, r := range rows {
		if _, seen := byDay[r.Day]; !seen {
			days = append(days, r.Day)
		}
		byDay[r.Day] = append(byDay[r.Day], r)
	}

	// Ordered by day rather than score, so the flakiest is found by scanning.
	flakiestRow := rows[0]
	for _, r := range rows {
		if r.FlakeScore > flakiestRow.FlakeScore {
			flakiestRow = r
		}
	}
	distinct := distinctFlakyTests(rows)

	doc.Headline = Int(int64(distinct)) + " flaky test(s) over " +
		Int(int64(len(days))) + " day(s); flakiest " + flakiestRow.TestName

	doc.Sections = append(doc.Sections, Section{
		Heading: "Summary",
		Metrics: []Metric{
			{Name: "Flaky tests listed", Value: Int(int64(distinct))},
			{Name: "Days with flakes", Value: Int(int64(len(days)))},
			{Name: "Flakiest test", Value: flakiestRow.TestName},
		},
		Notes: append(flakyScoreNotes(p, "within a day"),
			"Each test is scored within its own day, so a test that fails a "+
				"little every day ranks low here and can still top the "+
				FlakyRollupName+" report."),
	})

	for _, day := range days {
		doc.Sections = append(doc.Sections, Section{
			Heading: day,
			Table:   flakyTable(byDay[day]),
			Notes:   flakyCapNote(byDay[day], p.Top, "per-day "),
		})
	}

	return doc, nil
}

// decodeFlakyDailyRows reads the result into typed rows, day included.
func decodeFlakyDailyRows(rs *athena.Result) ([]flakyRow, error) {
	scanner := flakyScanner(rs).
		Str("day", func(r *flakyRow, v string) { r.Day = v })
	return collectFlakyRows(scanner, rs)
}
