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

// fakeExecutor records statements instead of running them.
type fakeExecutor struct {
	statements []string
	failOn     func(statement string) error
}

func (f *fakeExecutor) Execute(_ context.Context, statement string) (*athena.Result, error) {
	f.statements = append(f.statements, statement)

	if f.failOn != nil {
		if err := f.failOn(statement); err != nil {
			return nil, err
		}
	}
	return &athena.Result{
		QueryExecutionID: "test",
		DataScannedBytes: 1024,
		EngineTime:       time.Second,
	}, nil
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

	exec := &fakeExecutor{}
	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 13),
		To:             date(2026, time.September, 15),
	})
	require.NoError(t, err)

	require.Len(t, exec.statements, 3)

	assert.Contains(t, exec.statements[0], "'2026-09-13'")
	assert.Contains(t, exec.statements[2], "'2026-09-15'")
}

func TestRunSingleDay(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{}
	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 1),
		To:             date(2026, time.September, 1),
	})
	require.NoError(t, err)
	require.Len(t, exec.statements, 1)
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	t.Parallel()

	exec := &fakeExecutor{
		failOn: func(statement string) error {
			if strings.Contains(statement, "'2026-09-02'") {
				return trace.Errorf("boom")
			}
			return nil
		},
	}

	err := Run(context.Background(), exec, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 1),
		To:             date(2026, time.September, 5),
	})
	assert.Error(t, err)

	require.Len(t, exec.statements, 2)
}

func TestRunRejectsReversedPeriod(t *testing.T) {
	t.Parallel()

	err := Run(context.Background(), &fakeExecutor{}, Options{
		Database:       "db",
		TableMigration: testTable(),
		From:           date(2026, time.September, 5),
		To:             date(2026, time.September, 1),
	})
	assert.Error(t, err)
}
