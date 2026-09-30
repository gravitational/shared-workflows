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
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

const DefaultFailedRunsLimit = 100

// FailedRunParams identifies a test whose failing GitHub Actions runs should be listed.
type FailedRunParams struct {
	Classname string
	TestName  string
	Branches  []string
	Limit     int
}

// FailedRun is one GitHub Actions run containing a failed or errored testcase.
type FailedRun struct {
	Timestamp  string `json:"timestamp"`
	Repository string `json:"repository"`
	Workflow   string `json:"workflow"`
	Job        string `json:"job"`
	RunID      string `json:"run_id"`
	Attempt    string `json:"run_attempt"`
	GitRef     string `json:"git_ref"`
	GitSHA     string `json:"git_sha"`
	Classname  string `json:"package"`
	TestName   string `json:"test_name"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	GitHubURL  string `json:"github_url"`
}

type failedRunsTemplateData struct {
	Database       string
	MetaTable      string
	TestcasesTable string
	From           string
	To             string
	TargetBranches string
	TestPredicate  string
	Limit          int
}

// ExecuteFailedRuns queries GitHub Actions runs where the named test failed or errored.
func ExecuteFailedRuns(ctx context.Context, q athena.Executor, scope Scope, params FailedRunParams) ([]FailedRun, error) {
	stmt, err := FailedRunsStatement(scope, params)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	rs, err := q.Execute(ctx, stmt.SQL)
	if err != nil {
		return nil, trace.Wrap(err, "running failed runs query")
	}
	rows, err := DecodeFailedRuns(rs)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return rows, nil
}

// FailedRunsStatement renders the failed runs query.
func FailedRunsStatement(scope Scope, params FailedRunParams) (Statement, error) {
	if err := scope.checkAndSetDefaults(); err != nil {
		return Statement{}, trace.Wrap(err, "invalid scope configuration")
	}
	if params.TestName == "" {
		return Statement{}, trace.BadParameter("test name is required")
	}
	if params.Limit == 0 {
		params.Limit = DefaultFailedRunsLimit
	}
	if params.Limit < 1 || params.Limit > 1000 {
		return Statement{}, trace.BadParameter("limit must be between 1 and 1000, got %d", params.Limit)
	}
	if len(params.Branches) == 0 {
		params.Branches = DefaultFlakyBranches
	}
	for _, ref := range params.Branches {
		if !branchRefPattern.MatchString(ref) {
			return Statement{}, trace.BadParameter("invalid branch %q", ref)
		}
	}

	targetBranches := make([]string, 0, len(params.Branches))
	for _, branch := range params.Branches {
		targetBranches = append(targetBranches, "'"+branch+"'")
	}

	predicate := "t.test_name = '" + sqlString(params.TestName) + "'"
	if params.Classname != "" {
		predicate = "t.classname = '" + sqlString(params.Classname) + "' AND " + predicate
	}

	data := failedRunsTemplateData{
		Database:       scope.Database,
		MetaTable:      scope.Tables.Meta,
		TestcasesTable: scope.Tables.Testcases,
		From:           Day(scope.From),
		To:             Day(scope.To),
		TargetBranches: strings.Join(targetBranches, ", "),
		TestPredicate:  predicate,
		Limit:          params.Limit,
	}

	var buf strings.Builder
	if err := templates.ExecuteTemplate(&buf, "failed_runs.sql", data); err != nil {
		return Statement{}, trace.Wrap(err, "rendering failed runs template")
	}
	return Statement{SQL: strings.TrimRight(buf.String(), "\n")}, nil
}

// DecodeFailedRuns decodes the failed runs query result.
func DecodeFailedRuns(rs *athena.Result) ([]FailedRun, error) {
	scanner := athena.NewScanner[FailedRun](rs).
		Str("run_timestamp", func(r *FailedRun, v string) { r.Timestamp = v }).
		Str("repository", func(r *FailedRun, v string) { r.Repository = v }).
		Str("workflow", func(r *FailedRun, v string) { r.Workflow = v }).
		Str("job", func(r *FailedRun, v string) { r.Job = v }).
		Str("run_id", func(r *FailedRun, v string) { r.RunID = v }).
		Str("run_attempt", func(r *FailedRun, v string) { r.Attempt = v }).
		Str("git_ref", func(r *FailedRun, v string) { r.GitRef = v }).
		Str("git_sha", func(r *FailedRun, v string) { r.GitSHA = v }).
		Str("classname", func(r *FailedRun, v string) { r.Classname = v }).
		Str("test_name", func(r *FailedRun, v string) { r.TestName = v }).
		Str("status", func(r *FailedRun, v string) { r.Status = v }).
		Str("message", func(r *FailedRun, v string) { r.Message = v }).
		Str("github_url", func(r *FailedRun, v string) { r.GitHubURL = v })

	out := make([]FailedRun, 0, len(rs.Rows))
	for row, err := range scanner.Scan() {
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out = append(out, row)
	}
	return out, nil
}

func sqlString(s string) string { return strings.ReplaceAll(s, "'", "''") }

// FlakyResult is a JSON-friendly view of the flaky report table.
type FlakyResult struct {
	Rank       int64   `json:"rank"`
	Classname  string  `json:"package"`
	TestName   string  `json:"test_name"`
	Execs      int64   `json:"execs"`
	Fails      int64   `json:"fails"`
	FailPct    float64 `json:"fail_pct"`
	FlakeScore float64 `json:"flake_score"`
}

// ExecuteFlakyRows returns the flaky ranking rows without rendering a report document.
func ExecuteFlakyRows(ctx context.Context, q athena.Executor, scope Scope, params FlakyParams) ([]FlakyResult, error) {
	if err := scope.checkAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err, "invalid scope configuration")
	}
	statements, err := flakyQueries(scope, &params)
	if err != nil {
		return nil, trace.Wrap(err, "building flaky query")
	}
	rs, err := q.Execute(ctx, statements[flakyQuery].SQL)
	if err != nil {
		return nil, trace.Wrap(err, "running flaky query")
	}
	rows, err := decodeFlakyRows(rs)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	out := make([]FlakyResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, FlakyResult{
			Rank: r.Rank, Classname: r.Classname, TestName: r.TestName,
			Execs: r.Execs, Fails: r.Fails, FailPct: r.FailPct, FlakeScore: r.FlakeScore,
		})
	}
	return out, nil
}

func ParseDay(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, trace.BadParameter("date is required")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, trace.Wrap(err, "parsing date %q", s)
	}
	return t.UTC(), nil
}

func AtoiDefault(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	return n, nil
}
