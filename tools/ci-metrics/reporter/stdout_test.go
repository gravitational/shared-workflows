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

package reporter

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// testDocument is a small document exercising every field the text reporter
// renders.
func testDocument() *report.Document {
	return &report.Document{
		ID:       "flaky",
		Title:    "Top flaky tests",
		Subtitle: "db.testcases_v2_parquet 2026-09-01 .. 2026-09-02",
		Headline: "2 flaky test(s) over 1 day(s)",
		Sections: []report.Section{
			{
				Heading: "Summary",
				Metrics: []report.Metric{
					{Name: "Flaky tests", Value: "2"},
					{Name: "Worst", Value: "0.5000"},
				},
				Notes: []string{"Excludes tests with fewer than 10 executions."},
			},
			{
				Heading: "2026-09-02",
				Table: &report.Table{
					Columns: []report.Column{
						{Name: "#", Align: report.AlignRight},
						{Name: "TEST", Align: report.AlignLeft},
						{Name: "SCORE", Align: report.AlignRight},
					},
					Rows: []report.Row{
						{{Text: "1"}, {Text: "TestA"}, {Text: "0.5000"}},
						{{Text: "2"}, {Text: "TestWithALongName"}, {Text: "0.1000"}},
					},
					TotalRows: 2,
				},
			},
		},
		Meta: report.DocMeta{
			DataScannedBytes:  2048,
			QueryExecutionIDs: []string{"exec-1"},
		},
	}
}

func renderDoc(t *testing.T, doc *report.Document, maxRows int) string {
	t.Helper()

	var buf strings.Builder
	r := NewStdout("stdout", &buf, maxRows)
	require.Equal(t, "stdout", r.Name())
	require.NoError(t, r.Report(context.Background(), doc))
	require.NoError(t, r.Close())

	return buf.String()
}

func TestStdoutRendersOnlyTables(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 0)

	for _, want := range []string{"#", "TEST", "SCORE", "TestA", "TestWithALongName", "0.5000"} {
		assert.Contains(t, out, want)
	}
	for _, unwanted := range []string{
		"Top flaky tests",
		"db.testcases_v2_parquet",
		"2 flaky test(s)",
		"Summary",
		"Flaky tests",
		"Worst",
		"Excludes tests",
		"2026-09-02",
		"scanned",
	} {
		assert.NotContains(t, out, unwanted)
	}
}

func TestStdoutUsesTabsBetweenColumns(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 0)
	assert.Equal(t,
		"#\tTEST\tSCORE\n"+
			"1\tTestA\t0.5000\n"+
			"2\tTestWithALongName\t0.1000\n",
		out)
}

func TestStdoutSanitizesTabsAndNewlinesInCells(t *testing.T) {
	t.Parallel()

	doc := &report.Document{Sections: []report.Section{{Table: &report.Table{
		Columns: []report.Column{{Name: "VALUE"}},
		Rows:    []report.Row{{{Text: "one\ttwo\nthree"}}},
	}}}}

	assert.Equal(t, "VALUE\none two three\n", renderDoc(t, doc, 0))
}

func TestStdoutHasNoTrailingWhitespace(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 0)

	for i, line := range strings.Split(out, "\n") {
		assert.Equal(t, strings.TrimRight(line, " \t"), line,
			"line %d has trailing whitespace: %q", i, line)
	}
}

func TestStdoutTruncatesRows(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 1)

	assert.Contains(t, out, "TestA")
	assert.NotContains(t, out, "TestWithALongName", "second row should be dropped")
	assert.Contains(t, out, "showing 1 of 2", "truncation must be stated, not silent")
}

func TestStdoutSeparatesTables(t *testing.T) {
	t.Parallel()

	doc := testDocument()
	doc.Sections = append(doc.Sections, report.Section{Table: &report.Table{
		Columns: []report.Column{{Name: "OTHER"}},
		Rows:    []report.Row{{{Text: "value"}}},
	}})

	out := renderDoc(t, doc, 0)
	assert.Contains(t, out, "0.1000\n\nOTHER")
}

func TestStdoutEmptyDocument(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, &report.Document{Title: "Nothing"}, 0)
	assert.Empty(t, out)
}

func TestStdoutRejectsNilDocument(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	r := NewStdout("stdout", &buf, 0)
	assert.Error(t, r.Report(context.Background(), nil))
}
