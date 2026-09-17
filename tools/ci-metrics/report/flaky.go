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
	"embed"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

//go:embed sql/*.sql
var sqlFS embed.FS

var templates = template.Must(
	template.New("report").
		Option("missingkey=error").
		ParseFS(sqlFS, "sql/*.sql"),
)

// FlakyName is the registered name of the flaky test report.
const FlakyName = "flaky"

// flakyQuery is the key of the single statement the report runs.
const flakyQuery = "flaky"

func init() {
	Register(Definition{
		Name:      FlakyName,
		Summary:   "Rank the flakiest tests per day over a window",
		NewParams: func() any { return &FlakyParams{} },
		Queries:   flakyQueries,
		Render:    flakyRender,
	})
}

// FlakyParams tunes the flaky report.
type FlakyParams struct {
	// MinExecs is the minimum executions a test needs in a day before it is
	// considered at all, so a test that ran twice cannot top the table.
	MinExecs int `yaml:"min_execs"`
	// Smoothing is the K in the score's execs/(execs+K) term, which pulls
	// small samples towards zero. Larger values demand more evidence.
	Smoothing float64 `yaml:"smoothing"`
	// Top is how many tests to keep per day.
	Top int `yaml:"top"`
	// Branches are the refs whose runs count, as full refs such as
	// refs/heads/master. A merge-queue run targeting one of them counts too;
	// everything else, pull requests included, is excluded. Counts are summed
	// across all of them rather than reported per branch.
	Branches []string `yaml:"branches"`
}

var branchRefPattern = regexp.MustCompile(`^refs/heads/[A-Za-z0-9._][A-Za-z0-9._/-]*$`)

func (p *FlakyParams) checkAndSetDefaults() error {
	const (
		defaultFlakyMinExecs  = 10
		defaultFlakySmoothing = 20.0
		defaultFlakyTop       = 20
	)
	if p.MinExecs == 0 {
		p.MinExecs = defaultFlakyMinExecs
	}
	if p.Smoothing == 0 {
		p.Smoothing = defaultFlakySmoothing
	}
	if p.Top == 0 {
		p.Top = defaultFlakyTop
	}
	if len(p.Branches) == 0 {
		p.Branches = []string{
			"refs/heads/master",
			"refs/heads/main",
		}
	}

	if p.MinExecs < 2 {
		// A test needs at least one pass and one failure to be a flake, so
		// anything below two executions can never qualify.
		return trace.BadParameter("min_execs must be at least 2, got %d", p.MinExecs)
	}

	// Smoothing is the one parameter rendered into the statement as a
	// floating-point literal, and YAML accepts .nan and .inf as floats, which
	// would reach Athena as a bare identifier.
	if math.IsNaN(p.Smoothing) || math.IsInf(p.Smoothing, 0) {
		return trace.BadParameter("smoothing must be a finite number, got %v", p.Smoothing)
	}
	if p.Smoothing < 0 {
		return trace.BadParameter("smoothing must not be negative, got %v", p.Smoothing)
	}
	if p.Top < 1 {
		return trace.BadParameter("top must be at least 1, got %d", p.Top)
	}
	for _, ref := range p.Branches {
		if !branchRefPattern.MatchString(ref) {
			return trace.BadParameter(
				"invalid branch %q, want a full ref such as refs/heads/master", ref)
		}
	}
	return nil
}

// flakyBranchPredicate builds the ref filter for one table alias.
//
// Merge-queue refs are admitted by prefix rather than per branch, so a queued
// run counts even when it targets a branch outside Branches; the statement
// then normalises its ref back to that branch, which becomes a grouping key of
// its own. That is deliberately the hand-run Athena query's behaviour, kept so
// the two can be compared directly. Filtering the queue per branch instead is
// a change here rather than in the template.
//
// The refs are interpolated as string literals, which is safe because
// [FlakyParams.checkAndSetDefaults] has already held them to
// [branchRefPattern].
func flakyBranchPredicate(alias string, branches []string) (string, error) {
	const mergeQueueRefPrefix = "refs/heads/gh-readonly-queue"

	if len(branches) == 0 {
		return "", trace.BadParameter("no branches configured")
	}

	quoted := make([]string, 0, len(branches))
	for _, ref := range branches {
		if !branchRefPattern.MatchString(ref) {
			return "", trace.BadParameter("invalid branch %q", ref)
		}
		quoted = append(quoted, "'"+ref+"'")
	}

	// The continuation aligns under the opening paren as the template indents it.
	return fmt.Sprintf("(%s.git_ref IN (%s)\n           OR %s.git_ref LIKE '%s/%%')",
		alias, strings.Join(quoted, ", "), alias, mergeQueueRefPrefix), nil
}

// flakyParams recovers the typed params from the registry's any.
func flakyParams(params any) (*FlakyParams, error) {
	p, ok := params.(*FlakyParams)
	if !ok {
		return nil, trace.BadParameter("invalid parameter type %T", params)
	}
	if err := p.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return p, nil
}

// flakyQueries renders the statement for the window in scope.
func flakyQueries(scope Scope, params any) (map[string]Statement, error) {
	p, err := flakyParams(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if err := scope.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	if scope.From.After(scope.To) {
		return nil, trace.BadParameter("period start %s is after period end %s",
			Day(scope.From), Day(scope.To))
	}

	branchPredicate, err := flakyBranchPredicate("m", p.Branches)
	if err != nil {
		return nil, trace.Wrap(err, "building branch predicate")
	}

	var buf strings.Builder
	// Identifiers are interpolated textually and so are validated by
	// [Scope.checkAndSetDefaults]; the remaining values are numeric and
	// formatted here, so no caller-supplied string reaches the statement.
	err = templates.ExecuteTemplate(&buf, FlakyName+".sql", struct {
		Database        string
		MetaTable       string
		TestcasesTable  string
		From            string
		To              string
		BranchPredicate string
		MinExecs        int
		Smoothing       string
		Top             int
	}{
		Database:        scope.Database,
		MetaTable:       scope.Tables.Meta,
		TestcasesTable:  scope.Tables.Testcases,
		From:            Day(scope.From),
		To:              Day(scope.To),
		BranchPredicate: branchPredicate,
		MinExecs:        p.MinExecs,
		Smoothing:       strconv.FormatFloat(p.Smoothing, 'f', 1, 64),
		Top:             p.Top,
	})
	if err != nil {
		return nil, trace.Wrap(err, "rendering %s template", FlakyName)
	}

	return map[string]Statement{
		flakyQuery: {SQL: strings.TrimRight(buf.String(), "\n")},
	}, nil
}

// flakyRow is one scored test on one day.
type flakyRow struct {
	Day        string
	Rank       int64
	Classname  string
	TestName   string
	Execs      int64
	Fails      int64
	FailPct    float64
	FlakeScore float64
}

// flakyRender groups the scored rows into one section per day.
func flakyRender(scope Scope, params any, results map[string]*athena.Result) (*Document, error) {
	p, err := flakyParams(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	rs, ok := results[flakyQuery]
	if !ok || rs == nil {
		return nil, trace.BadParameter("flaky report got no %q result", flakyQuery)
	}

	doc := &Document{
		ID:       FlakyName,
		Title:    "Top flaky tests",
		Subtitle: scope.Database + "." + scope.Tables.Testcases + " | " + Window(scope.From, scope.To),
	}

	rows, err := decodeFlakyRows(rs)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(rows) == 0 {
		doc.Headline = "No flaky tests found in the window."
		doc.Sections = []Section{{
			Heading: "Summary",
			Notes: []string{
				"No test both passed and failed on the same day with at least " +
					strconv.Itoa(p.MinExecs) + " executions.",
			},
		}}
		return doc, nil
	}

	// The query orders by day descending, so grouping in arrival order keeps
	// the most recent day first without re-sorting.
	distinct := map[string]struct{}{}
	var days []string
	byDay := map[string][]flakyRow{}
	worst := rows[0]

	for _, r := range rows {
		if _, seen := byDay[r.Day]; !seen {
			days = append(days, r.Day)
		}
		byDay[r.Day] = append(byDay[r.Day], r)
		distinct[r.Classname+"."+r.TestName] = struct{}{}
		if r.FlakeScore > worst.FlakeScore {
			worst = r
		}
	}

	doc.Headline = Int(int64(len(distinct))) + " flaky test(s) over " +
		Int(int64(len(days))) + " day(s); worst " + worst.TestName +
		" (score " + Score(worst.FlakeScore) + ")"

	doc.Sections = append(doc.Sections, Section{
		Heading: "Summary",
		Metrics: []Metric{
			{Name: "Flaky tests listed", Value: Int(int64(len(distinct)))},
			{Name: "Days with flakes", Value: Int(int64(len(days)))},
			{Name: "Worst flake score", Value: Score(worst.FlakeScore)},
			{Name: "Worst test", Value: worst.TestName},
		},
		Notes: []string{
			"Score is 4*p*(1-p)*execs/(execs+" + strconv.FormatFloat(p.Smoothing, 'f', -1, 64) +
				"), approaching 1.0 for a test that fails half the time with a large sample.",
			"Tests with fewer than " + strconv.Itoa(p.MinExecs) +
				" executions in a day are excluded, as are tests that always passed or always failed.",
			"Scoring is per day, so a test that fails every run one day and passes every run the " +
				"next is excluded on both.",
		},
	})

	for _, day := range days {
		doc.Sections = append(doc.Sections, flakySection(day, byDay[day], p.Top))
	}

	return doc, nil
}

// flakySection builds the table for one day.
func flakySection(day string, rows []flakyRow, top int) Section {
	table := &Table{
		Columns: []Column{
			{Name: "#", Align: AlignRight},
			{Name: "TEST", Align: AlignLeft},
			{Name: "CLASS", Align: AlignLeft},
			{Name: "EXECS", Align: AlignRight},
			{Name: "FAILS", Align: AlignRight},
			{Name: "FAIL%", Align: AlignRight},
			{Name: "SCORE", Align: AlignRight},
		},
		Rows:      make([]Row, 0, len(rows)),
		TotalRows: len(rows),
	}

	for _, r := range rows {
		table.Rows = append(table.Rows, Row{
			{Text: Int(r.Rank)},
			{Text: r.TestName},
			{Text: r.Classname},
			{Text: Int(r.Execs)},
			{Text: Int(r.Fails)},
			{Text: Pct(r.FailPct)},
			{Text: Score(r.FlakeScore)},
		})
	}

	section := Section{
		Heading: day,
		Table:   table,
	}

	if len(rows) >= top {
		section.Notes = []string{
			"At the query's per-day cap of " + strconv.Itoa(top) + "; there may be more.",
		}
	}

	return section
}

// decodeFlakyRows reads the result into typed rows.
func decodeFlakyRows(rs *athena.Result) ([]flakyRow, error) {
	scanner := athena.NewScanner[flakyRow](rs).
		Str("day", func(r *flakyRow, v string) { r.Day = v }).
		Int64("rn", func(r *flakyRow, v int64) { r.Rank = v }).
		Str("classname", func(r *flakyRow, v string) { r.Classname = v }).
		Str("test_name", func(r *flakyRow, v string) { r.TestName = v }).
		Int64("execs", func(r *flakyRow, v int64) { r.Execs = v }).
		Int64("fails", func(r *flakyRow, v int64) { r.Fails = v }).
		Float64("fail_pct", func(r *flakyRow, v float64) { r.FailPct = v }).
		Float64("flake_score", func(r *flakyRow, v float64) { r.FlakeScore = v })

	out := make([]flakyRow, 0, len(rs.Rows))
	for row, err := range scanner.Scan() {
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out = append(out, row)
	}

	return out, nil
}
