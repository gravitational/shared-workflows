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

func TestFlakyReportsAreRegisteredSeparately(t *testing.T) {
	t.Parallel()

	// Each name has to resolve on its own so either can be run without the other.
	for _, name := range []string{FlakyRollupName, FlakyDailyName} {
		def, ok := Get(name)
		require.True(t, ok, "report %q is not registered", name)
		assert.Equal(t, name, def.Name)
		assert.NotEmpty(t, def.Summary)
	}

	// The combined report was replaced by the two above.
	_, ok := Get("flaky")
	assert.False(t, ok, "the combined flaky report should no longer be registered")
}

func TestFlakyQueriesRunOneStatementEach(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name     string
		key      string
		wants    []string
		notWants []string
	}{
		"rollup": {
			name:     FlakyRollupName,
			key:      flakyRollupQuery,
			wants:    []string{"GROUP BY 1, 2, 3", "ORDER BY rn"},
			notWants: []string{"PARTITION BY day"},
		},
		"daily": {
			name:     FlakyDailyName,
			key:      flakyDailyQuery,
			wants:    []string{"GROUP BY 1, 2, 3, 4", "PARTITION BY day", "ORDER BY day DESC, rn"},
			notWants: nil,
		},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			def, ok := Get(tt.name)
			require.True(t, ok)

			stmts, err := def.Queries(testScope(), def.NewParams())
			require.NoError(t, err)

			// One statement means one scan.
			require.Len(t, stmts, 1)
			require.Contains(t, stmts, tt.key)

			sql := stmts[tt.key].SQL
			assert.Contains(t, sql, "example_db.meta_p")
			assert.Contains(t, sql, "example_db.testcases_p")
			assert.Contains(t, sql, "'2026-09-01' AND '2026-09-03'")
			assert.Contains(t, sql, "'refs/heads/master'")
			assert.Contains(t, sql, "refs/heads/gh-readonly-queue/%")

			for _, want := range tt.wants {
				assert.Contains(t, sql, want)
			}
			for _, notWant := range tt.notWants {
				assert.NotContains(t, sql, notWant)
			}
		})
	}
}

func TestFlakyQueriesRejectBadParams(t *testing.T) {
	t.Parallel()

	for _, name := range []string{FlakyRollupName, FlakyDailyName} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			def, ok := Get(name)
			require.True(t, ok)

			_, err := def.Queries(testScope(), &FlakyParams{MinExecs: 1})
			assert.Error(t, err, "min_execs below 2 should be rejected")

			_, err = def.Queries(testScope(), &FlakyParams{
				Branches: []string{"master; DROP TABLE x"},
			})
			assert.Error(t, err, "a branch that is not a full ref should be rejected")

			_, err = def.Render(testScope(), def.NewParams(), map[string]*athena.Result{})
			assert.Error(t, err, "a missing result should be an error, not an empty document")
		})
	}
}

func TestFlakyRollupRender(t *testing.T) {
	t.Parallel()

	rs := &athena.Result{
		Columns: []string{"rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"},
		Rows: []athena.Row{
			cells("1", "pkg/auth", "TestLogin", "120", "40", "33.33", "0.7634"),
			cells("2", "pkg/auth", "TestLogout", "100", "10", "10.00", "0.3000"),
		},
	}

	doc, err := flakyRollupRender(testScope(), &FlakyParams{}, map[string]*athena.Result{
		flakyRollupQuery: rs,
	})
	require.NoError(t, err)

	assert.Equal(t, FlakyRollupName, doc.ID)
	assert.Contains(t, doc.Headline, "flakiest TestLogin")

	// Summary, then the one ranked table.
	require.Len(t, doc.Sections, 2)
	table := doc.Sections[1].Table
	require.NotNil(t, table)
	assert.Equal(t, 2, table.TotalRows)
	assert.Equal(t, "TestLogin", table.Rows[0][1].Text)
	assert.Equal(t, "33.33%", table.Rows[0][5].Text)
}

func TestFlakyDailyRender(t *testing.T) {
	t.Parallel()

	rs := &athena.Result{
		Columns: []string{"day", "rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"},
		Rows: []athena.Row{
			cells("2026-09-03", "1", "pkg/auth", "TestLogout", "40", "8", "20.00", "0.2133"),
			cells("2026-09-02", "1", "pkg/auth", "TestLogin", "40", "20", "50.00", "0.6667"),
			cells("2026-09-02", "2", "pkg/auth", "TestLogout", "40", "4", "10.00", "0.1200"),
		},
	}

	doc, err := flakyDailyRender(testScope(), &FlakyParams{}, map[string]*athena.Result{
		flakyDailyQuery: rs,
	})
	require.NoError(t, err)

	assert.Equal(t, FlakyDailyName, doc.ID)
	// Rows arrive ordered by day, not by score.
	assert.Contains(t, doc.Headline, "flakiest TestLogin")
	assert.Contains(t, doc.Headline, "2 flaky test(s) over 2 day(s)")

	// Summary, then one section per day.
	require.Len(t, doc.Sections, 3)
	assert.Equal(t, "2026-09-03", doc.Sections[1].Heading)
	assert.Equal(t, "2026-09-02", doc.Sections[2].Heading)
	require.NotNil(t, doc.Sections[2].Table)
	assert.Equal(t, 2, doc.Sections[2].Table.TotalRows)
}

func TestFlakyRenderEmptyResult(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		render  func(Scope, any, map[string]*athena.Result) (*Document, error)
		key     string
		columns []string
	}{
		"rollup": {
			render:  flakyRollupRender,
			key:     flakyRollupQuery,
			columns: []string{"rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"},
		},
		"daily": {
			render:  flakyDailyRender,
			key:     flakyDailyQuery,
			columns: []string{"day", "rn", "classname", "test_name", "execs", "fails", "fail_pct", "flake_score"},
		},
	}

	for label, tt := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			doc, err := tt.render(testScope(), &FlakyParams{}, map[string]*athena.Result{
				tt.key: {Columns: tt.columns},
			})
			require.NoError(t, err)

			assert.Contains(t, doc.Headline, "No flaky tests")
			require.Len(t, doc.Sections, 1)
			assert.Nil(t, doc.Sections[0].Table)
			assert.NotEmpty(t, doc.Sections[0].Notes)
		})
	}
}

func TestFlakyCapNote(t *testing.T) {
	t.Parallel()

	rows := make([]flakyRow, 3)

	assert.Empty(t, flakyCapNote(rows, 4, ""), "below the cap there is nothing to warn about")

	note := flakyCapNote(rows, 3, "per-day ")
	require.Len(t, note, 1)
	assert.True(t, strings.Contains(note[0], "per-day cap of 3"), "got %q", note[0])
}
