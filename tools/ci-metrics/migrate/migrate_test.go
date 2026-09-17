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
	"strings"
	"testing"
	"time"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/athena"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func testTable() TableMigration {
	return TableMigration{
		Type:        "testcases",
		Source:      "testcases_jsonl",
		Destination: "testcases_parquet",
	}
}

// mockExecutor stands in for Athena. The statements it was given are read back
// from mock.Calls, which preserves the order they arrived in.
type mockExecutor struct {
	mock.Mock
}

var _ athena.Executor = (*mockExecutor)(nil)

func (m *mockExecutor) Execute(ctx context.Context, statement string) (*athena.Result, error) {
	ret := m.Called(ctx, statement)

	// A stubbed failure returns a nil result, which is an untyped nil in the
	// return arguments, so the assertion has to be the two-value form.
	result, _ := ret.Get(0).(*athena.Result)
	return result, ret.Error(1)
}

// statement returns the SQL passed to the nth Execute call.
func (m *mockExecutor) statement(t *testing.T, n int) string {
	t.Helper()

	require.Greater(t, len(m.Calls), n, "Execute was not called %d times", n+1)
	return m.Calls[n].Arguments.String(1)
}

// testResult is what a successful Execute returns. The values are arbitrary
// but non-zero, so a test asserting on them cannot pass against a zero value.
func testResult() *athena.Result {
	return &athena.Result{
		QueryExecutionID: "test",
		DataScannedBytes: 1024,
		EngineTime:       time.Second,
	}
}

func TestRender(t *testing.T) {
	t.Parallel()

	got, err := render("db", testTable(), date(2026, time.September, 14))
	require.NoError(t, err)

	assert.Contains(t, got, "INSERT INTO db.testcases_parquet")
	assert.Contains(t, got, "FROM db.testcases_jsonl s")

	assert.Contains(t, got, "(s.year = '2026' AND s.month = '09' AND s.day = '14')")
	assert.NotContains(t, got, " OR ")

	assert.Contains(t, got, "s.repository,\n       s.year || '-' || s.month || '-' || s.day AS dt")

	assert.Contains(t, got, "d.dt = '2026-09-14'")
	assert.Contains(t, got, "d.meta_id = s.meta_id")
}

func TestRenderEveryType(t *testing.T) {
	t.Parallel()

	for _, typ := range Types() {
		t.Run(typ, func(t *testing.T) {
			got, err := render("db", TableMigration{
				Type:        typ,
				Source:      "src_table",
				Destination: "dst_table",
			}, date(2026, time.January, 2))
			require.NoError(t, err)

			assert.Contains(t, got, "INSERT INTO db.dst_table")
			assert.Contains(t, got, "FROM db.src_table s")
			assert.Contains(t, got, "d.dt = '2026-01-02'")
			assert.Contains(t, got, "s.month = '01'")
			assert.Contains(t, got, "s.day = '02'")
		})
	}
}

func TestOptionsValidation(t *testing.T) {
	t.Parallel()

	for name, opts := range map[string]Options{
		"bad database name (numerical prefix)": {
			Database:       "1db",
			TableMigration: TableMigration{Type: "a", Source: "b", Destination: "c"},
		},
		"bad database name (dashes)": {
			Database:       "db-1",
			TableMigration: TableMigration{Type: "a", Source: "b", Destination: "c"},
		},
		"bad database name (injection)": {
			Database:       "db; DROP TABLE x",
			TableMigration: TableMigration{Type: "a", Source: "b", Destination: "c"},
		},
		"unknown type": {
			Database:       "ok",
			TableMigration: TableMigration{Type: "nonesuch", Source: "a", Destination: "b"},
		},
		"empty type": {
			Database:       "ok",
			TableMigration: TableMigration{Source: "a", Destination: "b"}},
		"bad source": {
			Database:       "ok",
			TableMigration: TableMigration{Type: "meta", Source: "a-table", Destination: "b"}},
		"injected source": {
			Database:       "ok",
			TableMigration: TableMigration{Type: "meta", Source: "a; DROP TABLE x", Destination: "b"}},
		"bad destination": {
			Database:       "ok",
			TableMigration: TableMigration{Type: "meta", Source: "a", Destination: ""}},
		"reads what it write": {
			Database:       "ok",
			TableMigration: TableMigration{Type: "meta", Source: "same", Destination: "same"}},
	} {
		assert.Error(t, opts.checkAndSetDefaults(), "%s should be rejected", name)
	}

}

func TestRunOneStatementPerDay(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	exec.On("Execute", mock.Anything, mock.Anything).Return(testResult(), nil)

	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 13),
		To:             date(2026, time.September, 15),
	})
	require.NoError(t, err)

	exec.AssertNumberOfCalls(t, "Execute", 3)

	assert.Contains(t, exec.statement(t, 0), "'2026-09-13'")
	assert.Contains(t, exec.statement(t, 2), "'2026-09-15'")
}

func TestRunSingleDay(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	exec.On("Execute", mock.Anything, mock.Anything).Return(testResult(), nil)

	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 1),
		To:             date(2026, time.September, 1),
	})
	require.NoError(t, err)
	exec.AssertNumberOfCalls(t, "Execute", 1)
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	t.Parallel()

	exec := &mockExecutor{}
	// Registered before the catch-all: testify matches against the first
	// expectation whose arguments satisfy the call, so the specific one has to
	// come first or it would never be reached.
	exec.On("Execute", mock.Anything, mock.MatchedBy(func(statement string) bool {
		return strings.Contains(statement, "'2026-09-02'")
	})).Return(nil, trace.Errorf("boom"))
	exec.On("Execute", mock.Anything, mock.Anything).Return(testResult(), nil)

	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 1),
		To:             date(2026, time.September, 5),
	})
	assert.Error(t, err)

	// The first day succeeds and the second fails, so a fifth day's worth of
	// statements would mean Run carried on past the error.
	exec.AssertNumberOfCalls(t, "Execute", 2)
}

func TestRunRejectsReversedPeriod(t *testing.T) {
	t.Parallel()

	// No expectations are set, so any call would panic as unexpected: the
	// window has to be rejected before a statement is built.
	exec := &mockExecutor{}

	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 5),
		To:             date(2026, time.September, 1),
	})
	assert.Error(t, err)

	exec.AssertNotCalled(t, "Execute", mock.Anything, mock.Anything)
}
