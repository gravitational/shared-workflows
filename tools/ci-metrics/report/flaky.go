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

const (
	// flakyDailyQuery is the key of the statement that ranks tests within
	// each day of the window.
	flakyDailyQuery = "daily"
	// flakyRollupQuery is the key of the statement that ranks tests over the
	// window as a whole.
	flakyRollupQuery = "rollup"
)

func init() {
	Register(Definition{
		Name:      FlakyName,
		Summary:   "Rank the flakiest tests over a window, as a whole and per day",
		NewParams: func() any { return &FlakyParams{} },
		Queries:   flakyQueries,
		Render:    flakyRender,
	})
}

// FlakyParams tunes the flaky report.
type FlakyParams struct {
	// MinExecs is the minimum executions a test needs before it is considered
	// at all, so a test that ran twice cannot top the table.
	MinExecs int `yaml:"min_execs"`
	// Smoothing is the K in the score's execs/(execs+K) term, which pulls
	// small samples towards zero. Larger values demand more evidence.
	Smoothing float64 `yaml:"smoothing"`
	// Top is how many tests to keep, applied to the window rollup and to each
	// day separately.
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

// flakyTemplateData is what both flaky statements are rendered from. They
// differ only in how they group, so they take identical inputs.
type flakyTemplateData struct {
	Database        string
	MetaTable       string
	TestcasesTable  string
	From            string
	To              string
	BranchPredicate string
	MinExecs        int
	Smoothing       string
	Top             int
}

// flakyQueries renders the statements for the window in scope: one ranking
// tests within each day, one ranking them over the window as a whole. Two
// statements means two scans of the same partitions.
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

	// Identifiers are interpolated textually and so are validated by
	// [Scope.checkAndSetDefaults]; the remaining values are numeric and
	// formatted here, so no caller-supplied string reaches the statement.
	data := flakyTemplateData{
		Database:        scope.Database,
		MetaTable:       scope.Tables.Meta,
		TestcasesTable:  scope.Tables.Testcases,
		From:            Day(scope.From),
		To:              Day(scope.To),
		BranchPredicate: branchPredicate,
		MinExecs:        p.MinExecs,
		Smoothing:       strconv.FormatFloat(p.Smoothing, 'f', 1, 64),
		Top:             p.Top,
	}

	out := make(map[string]Statement, 2)
	for key, tmpl := range map[string]string{
		flakyDailyQuery:  "flaky.sql",
		flakyRollupQuery: "flaky_rollup.sql",
	} {
		sql, err := renderFlakyTemplate(tmpl, data)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[key] = Statement{SQL: sql}
	}

	return out, nil
}

// renderFlakyTemplate renders one of the report's templates by file name.
func renderFlakyTemplate(name string, data flakyTemplateData) (string, error) {
	var buf strings.Builder
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", trace.Wrap(err, "rendering %s template", name)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
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

// flakyRollupRow is one scored test over the whole window.
type flakyRollupRow struct {
	Rank       int64
	Classname  string
	TestName   string
	Execs      int64
	Fails      int64
	FailPct    float64
	FlakeScore float64
}

// flakyRender builds the window rollup, then one section per day. The rollup
// leads because a test that fails a little every day scores poorly in every
// daily table and still tops the window.
func flakyRender(scope Scope, params any, results map[string]*athena.Result) (*Document, error) {
	p, err := flakyParams(params)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	dailyRS, ok := results[flakyDailyQuery]
	if !ok || dailyRS == nil {
		return nil, trace.BadParameter("flaky report got no %q result", flakyDailyQuery)
	}
	rollupRS, ok := results[flakyRollupQuery]
	if !ok || rollupRS == nil {
		return nil, trace.BadParameter("flaky report got no %q result", flakyRollupQuery)
	}

	doc := &Document{
		ID:       FlakyName,
		Title:    "Top flaky tests",
		Subtitle: scope.Database + "." + scope.Tables.Testcases + " | " + Window(scope.From, scope.To),
	}

	rows, err := decodeFlakyRows(dailyRS)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	rollup, err := decodeFlakyRollupRows(rollupRS)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(rows) == 0 && len(rollup) == 0 {
		doc.Headline = "No flaky tests found in the window."
		doc.Sections = []Section{{
			Heading: "Summary",
			Notes: []string{
				"No test both passed and failed, within a day or across the window, " +
					"with at least " + strconv.Itoa(p.MinExecs) + " executions.",
			},
		}}
		return doc, nil
	}

	// The daily query orders by day descending, so grouping in arrival order
	// keeps the most recent day first without re-sorting.
	distinct := map[string]struct{}{}
	var days []string
	byDay := map[string][]flakyRow{}

	for _, r := range rows {
		if _, seen := byDay[r.Day]; !seen {
			days = append(days, r.Day)
		}
		byDay[r.Day] = append(byDay[r.Day], r)
		distinct[r.Classname+"."+r.TestName] = struct{}{}
	}

	// The rollup is already ordered by score, so its first row is the worst
	// test of the window. The daily rows are only a fallback: a populated
	// daily table implies a populated rollup, but the two are separate
	// statements and nothing here enforces that.
	worst := ""
	switch {
	case len(rollup) > 0:
		worst = rollup[0].TestName
	case len(rows) > 0:
		worstRow := rows[0]
		for _, r := range rows {
			if r.FlakeScore > worstRow.FlakeScore {
				worstRow = r
			}
		}
		worst = worstRow.TestName
	}

	doc.Headline = Int(int64(len(distinct))) + " flaky test(s) over " +
		Int(int64(len(days))) + " day(s); worst " + worst

	doc.Sections = append(doc.Sections, Section{
		Heading: "Summary",
		Metrics: []Metric{
			{Name: "Flaky tests listed", Value: Int(int64(len(distinct)))},
			{Name: "Days with flakes", Value: Int(int64(len(days)))},
			{Name: "Worst test", Value: worst},
		},
		Notes: []string{
			"Score is 4*p*(1-p)*execs/(execs+" + strconv.FormatFloat(p.Smoothing, 'f', -1, 64) +
				"), approaching 1.0 for a test that fails half the time with a large sample.",
			"Tests with fewer than " + strconv.Itoa(p.MinExecs) +
				" executions are excluded, as are tests that always passed or always failed.",
			"The rollup scores each test over the window's totals; the daily tables score it " +
				"within each day, so a test that fails every run one day and passes every run " +
				"the next is excluded from both days but can still appear in the rollup.",
		},
	})

	doc.Sections = append(doc.Sections, flakyRollupSection(scope, rollup, p.Top))

	for _, day := range days {
		doc.Sections = append(doc.Sections, flakySection(day, byDay[day], p.Top))
	}

	return doc, nil
}

// flakyRollupSection builds the table for the window as a whole.
func flakyRollupSection(scope Scope, rows []flakyRollupRow, top int) Section {
	section := Section{
		Heading: "Window rollup | " + Window(scope.From, scope.To),
	}

	if len(rows) == 0 {
		section.Notes = []string{
			"No test qualified over the window's totals.",
		}
		return section
	}

	table := &Table{
		Columns: []Column{
			{Name: "#", Align: AlignRight},
			{Name: "TEST", Align: AlignLeft},
			{Name: "CLASS", Align: AlignLeft},
			{Name: "EXECS", Align: AlignRight},
			{Name: "FAILS", Align: AlignRight},
			{Name: "FAIL%", Align: AlignRight},
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
		})
	}

	section.Table = table

	if len(rows) >= top {
		section.Notes = []string{
			"At the query's cap of " + strconv.Itoa(top) + "; there may be more.",
		}
	}

	return section
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

// decodeFlakyRollupRows reads the window rollup result into typed rows.
func decodeFlakyRollupRows(rs *athena.Result) ([]flakyRollupRow, error) {
	scanner := athena.NewScanner[flakyRollupRow](rs).
		Int64("rn", func(r *flakyRollupRow, v int64) { r.Rank = v }).
		Str("classname", func(r *flakyRollupRow, v string) { r.Classname = v }).
		Str("test_name", func(r *flakyRollupRow, v string) { r.TestName = v }).
		Int64("execs", func(r *flakyRollupRow, v int64) { r.Execs = v }).
		Int64("fails", func(r *flakyRollupRow, v int64) { r.Fails = v }).
		Float64("fail_pct", func(r *flakyRollupRow, v float64) { r.FailPct = v }).
		Float64("flake_score", func(r *flakyRollupRow, v float64) { r.FlakeScore = v })

	out := make([]flakyRollupRow, 0, len(rs.Rows))
	for row, err := range scanner.Scan() {
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out = append(out, row)
	}

	return out, nil
}
