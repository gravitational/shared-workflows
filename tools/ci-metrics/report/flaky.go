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

// FlakyParams tunes both flaky reports. Only the grouping differs between
// them, so the same block on each gives comparable numbers.
type FlakyParams struct {
	// MinExecs is the minimum executions a test needs before it is considered
	// at all, so a test that ran twice cannot top the table. Counted within
	// the day by flaky_daily and over the window by flaky_rollup.
	MinExecs int `yaml:"min_execs"`
	// Smoothing is the K in the score's execs/(execs+K) term, which pulls
	// small samples towards zero. Larger values demand more evidence.
	Smoothing float64 `yaml:"smoothing"`
	// Top is how many tests to keep: the top of the window for flaky_rollup,
	// the top of each day for flaky_daily.
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

// flakyStatement renders the single statement a flaky report runs.
func flakyStatement(scope Scope, params any, queryKey, templateName string) (map[string]Statement, error) {
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
	// [Scope.checkAndSetDefaults]; everything else is numeric, so no
	// caller-supplied string reaches the statement.
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

	sql, err := renderFlakyTemplate(templateName, data)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return map[string]Statement{queryKey: {SQL: sql}}, nil
}

// renderFlakyTemplate renders one of the family's templates by file name.
func renderFlakyTemplate(name string, data flakyTemplateData) (string, error) {
	var buf strings.Builder
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", trace.Wrap(err, "rendering %s template", name)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// flakyResult pulls the one result a flaky report expects out of the map.
func flakyResult(results map[string]*athena.Result, queryKey, reportName string) (*athena.Result, error) {
	rs, ok := results[queryKey]
	if !ok || rs == nil {
		return nil, trace.BadParameter("%s report got no %q result", reportName, queryKey)
	}
	return rs, nil
}

// flakyRow is one scored test. Day is set only by flaky_daily, which buckets
// by day; the rollup's single bucket is the window itself.
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

// flakyScoreNotes explains the score and the exclusions.
func flakyScoreNotes(p *FlakyParams, bucket string) []string {
	return []string{
		"Score is 4*p*(1-p)*execs/(execs+" + strconv.FormatFloat(p.Smoothing, 'f', -1, 64) +
			"), approaching 1.0 for a test that fails half the time with a large sample.",
		"Tests with fewer than " + strconv.Itoa(p.MinExecs) + " executions " + bucket +
			" are excluded, as are tests that always passed or always failed " + bucket + ".",
	}
}

// flakyTable builds the ranked table the two reports share. Neither renders
// [flakyRow.Day]: the rollup has none and the daily sections group by it.
func flakyTable(rows []flakyRow) *Table {
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

	return table
}

// flakyScanner is the part of the scan both reports share.
func flakyScanner(rs *athena.Result) *athena.Scanner[flakyRow] {
	return athena.NewScanner[flakyRow](rs).
		Int64("rn", func(r *flakyRow, v int64) { r.Rank = v }).
		Str("classname", func(r *flakyRow, v string) { r.Classname = v }).
		Str("test_name", func(r *flakyRow, v string) { r.TestName = v }).
		Int64("execs", func(r *flakyRow, v int64) { r.Execs = v }).
		Int64("fails", func(r *flakyRow, v int64) { r.Fails = v }).
		Float64("fail_pct", func(r *flakyRow, v float64) { r.FailPct = v }).
		Float64("flake_score", func(r *flakyRow, v float64) { r.FlakeScore = v })
}

// collectFlakyRows drains a configured scanner into a slice.
func collectFlakyRows(scanner *athena.Scanner[flakyRow], rs *athena.Result) ([]flakyRow, error) {
	out := make([]flakyRow, 0, len(rs.Rows))
	for row, err := range scanner.Scan() {
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out = append(out, row)
	}
	return out, nil
}

// distinctFlakyTests counts the distinct tests named across rows.
func distinctFlakyTests(rows []flakyRow) int {
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		seen[r.Classname+"."+r.TestName] = struct{}{}
	}
	return len(seen)
}

// flakyCapNote warns that the ranking is truncated rather than exhausted.
func flakyCapNote(rows []flakyRow, top int, per string) []string {
	if len(rows) < top {
		return nil
	}
	return []string{"At the query's " + per + "cap of " + strconv.Itoa(top) + "; there may be more."}
}
