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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

// testScope is the scope the flaky tests render against.
func testScope() Scope {
	return Scope{
		Database: "example_db",
		Tables:   Tables{Meta: "meta_p", Testcases: "testcases_p"},
		From:     time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		To:       time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
	}
}

// cells turns a row of literals into the pointer cells a [athena.Result] holds.
func cells(values ...string) athena.Row {
	row := make(athena.Row, 0, len(values))
	for _, v := range values {
		row = append(row, &v)
	}
	return row
}

func TestFlakyReportIsRegistered(t *testing.T) {
	t.Parallel()

	def, ok := Get(FlakyName)
	require.True(t, ok, "report %q is not registered", FlakyName)
	assert.Equal(t, FlakyName, def.Name)
	assert.NotEmpty(t, def.Summary)

	// The per-day and rollup reports were merged into the one above.
	for _, name := range []string{"flaky_daily", "flaky_rollup"} {
		_, ok := Get(name)
		assert.False(t, ok, "%s should no longer be registered", name)
	}
}

func TestFlakyQueryRunsOneStatement(t *testing.T) {
	t.Parallel()

	def, ok := Get(FlakyName)
	require.True(t, ok)

	stmts, err := def.Queries(testScope(), def.NewParams())
	require.NoError(t, err)

	// One statement means one scan.
	require.Len(t, stmts, 1)
	require.Contains(t, stmts, flakyQuery)

	sql := stmts[flakyQuery].SQL
	assert.Contains(t, sql, "example_db.meta_p")
	assert.Contains(t, sql, "example_db.testcases_p")
	assert.Contains(t, sql, "'2026-09-01' AND '2026-09-03'")
	assert.Contains(t, sql, "'refs/heads/master'")
	assert.Contains(t, sql, "refs/heads/gh-readonly-queue/%")
	assert.Contains(t, sql, "GROUP BY 1, 2, 3")
	assert.Contains(t, sql, "ORDER BY rn")
	assert.NotContains(t, sql, "PARTITION BY day")
}

func TestFlakyQueryUsesParams(t *testing.T) {
	t.Parallel()

	stmts, err := flakyQueries(testScope(), &FlakyParams{
		MinExecs: 5,
		Top:      7,
		Branches: []string{"refs/heads/master", "refs/heads/branch/v18"},
	})
	require.NoError(t, err)

	sql := stmts[flakyQuery].SQL
	assert.Contains(t, sql, "execs >= 5")
	assert.Contains(t, sql, "rn <= 7")
	assert.Contains(t, sql, "IN ('refs/heads/master', 'refs/heads/branch/v18')")
}

func TestFlakyQueryRejectsBadParams(t *testing.T) {
	t.Parallel()

	def, ok := Get(FlakyName)
	require.True(t, ok)

	_, err := def.Queries(testScope(), &FlakyParams{MinExecs: 1})
	assert.Error(t, err, "min execs below 2 should be rejected")

	_, err = def.Queries(testScope(), &FlakyParams{Top: -1})
	assert.Error(t, err, "a negative top should be rejected")

	_, err = def.Queries(testScope(), &FlakyParams{
		Branches: []string{"master; DROP TABLE x"},
	})
	assert.Error(t, err, "a branch that is not a full ref should be rejected")

	_, err = def.Render(testScope(), def.NewParams(), map[string]*athena.Result{})
	assert.Error(t, err, "a missing result should be an error, not an empty document")
}

func TestFlakyRender(t *testing.T) {
	t.Parallel()

	rs := &athena.Result{
		Columns: []string{"rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"},
		Rows: []athena.Row{
			cells("1", "pkg/auth", "TestLogin", "120", "40", "33.33", "0.7634"),
			cells("2", "pkg/auth", "TestLogout", "100", "10", "10.00", "0.3000"),
		},
	}

	doc, err := flakyRender(testScope(), &FlakyParams{}, map[string]*athena.Result{
		flakyQuery: rs,
	})
	require.NoError(t, err)

	assert.Equal(t, FlakyName, doc.ID)
	assert.Contains(t, doc.Headline, "TestLogin")

	// Summary, then the one ranked table.
	require.Len(t, doc.Sections, 2)
	table := doc.Sections[1].Table
	require.NotNil(t, table)
	assert.Equal(t, 2, table.TotalRows)
	assert.Equal(t, "TestLogin", table.Rows[0][1].Text)
	assert.Equal(t, "33.33%", table.Rows[0][5].Text)
}

func TestFlakyRenderEmptyResult(t *testing.T) {
	t.Parallel()

	doc, err := flakyRender(testScope(), &FlakyParams{}, map[string]*athena.Result{
		flakyQuery: {Columns: []string{"rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"}},
	})
	require.NoError(t, err)

	assert.Contains(t, doc.Headline, "No flaky tests")
	require.Len(t, doc.Sections, 1)
	assert.Nil(t, doc.Sections[0].Table)
	assert.NotEmpty(t, doc.Sections[0].Notes)
}

func TestFlakyCapNote(t *testing.T) {
	t.Parallel()

	rows := make([]flakyRow, 3)

	assert.Empty(t, flakyCapNote(rows, 4), "below the cap there is nothing to warn about")

	note := flakyCapNote(rows, 3)
	require.Len(t, note, 1)
	assert.True(t, strings.Contains(note[0], "cap of 3"), "got %q", note[0])
}
