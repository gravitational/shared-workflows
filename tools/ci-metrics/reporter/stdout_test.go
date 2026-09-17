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
						{{Text: "1"}, {Text: "TestA"}, {Text: "0.5000", Severity: report.SeverityBad}},
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

func TestStdoutRendersEveryField(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 0)

	for _, want := range []string{
		"Top flaky tests",
		"db.testcases_v2_parquet 2026-09-01 .. 2026-09-02",
		"2 flaky test(s) over 1 day(s)",
		"Summary",
		"Flaky tests",
		"Worst",
		"Excludes tests with fewer than 10 executions.",
		"2026-09-02",
		"TestA",
		"TestWithALongName",
		"0.5000",
		"2.0 KiB scanned",
	} {
		assert.Contains(t, out, want)
	}
}

func TestStdoutRendersDetailSections(t *testing.T) {
	t.Parallel()

	// The text reporter is the verbose one: unlike a chat reporter it must not
	// drop [report.Detail] sections.
	out := renderDoc(t, testDocument(), 0)
	assert.Contains(t, out, "2026-09-02")
	assert.Contains(t, out, "TestWithALongName")
}

// columnEnd returns the offset one past the last rune of the trimmed line,
// which is where a right-aligned final column ends.
func columnEnd(line string) int {
	return len([]rune(strings.TrimRight(line, " ")))
}

func TestStdoutAlignsColumns(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, testDocument(), 0)

	// Assert the invariant rather than exact spacing: every line of the table
	// must end at the same offset, because the final column is right-aligned.
	// Checking the property instead of a literal keeps the test meaningful if
	// the fixture's widths change.
	var tableLines []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "SCORE") ||
			strings.Contains(line, "TestA") ||
			strings.Contains(line, "TestWithALongName") {
			tableLines = append(tableLines, line)
		}
	}
	require.Len(t, tableLines, 3, "header plus two rows")

	want := columnEnd(tableLines[0])
	for i, line := range tableLines {
		assert.Equal(t, want, columnEnd(line),
			"line %d should end where the right-aligned column ends: %q", i, line)
	}

	// The left-aligned column must start at the same offset on every row, so
	// the widest value determines the column and nothing is clipped.
	first := strings.Index(tableLines[1], "TestA")
	second := strings.Index(tableLines[2], "TestWithALongName")
	assert.Equal(t, first, second, "left-aligned column should start at a fixed offset")
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

func TestStdoutRendersMetricDeltas(t *testing.T) {
	t.Parallel()

	doc := &report.Document{
		Title: "Trends",
		Sections: []report.Section{{
			Heading: "Summary",
			Metrics: []report.Metric{
				{Name: "Failure rate", Value: "4.20%", Delta: &report.Delta{Text: "1.10pp", Up: true}},
				{Name: "Retry rate", Value: "2.00%", Delta: &report.Delta{Text: "0.30pp", Up: false}},
			},
		}},
	}

	out := renderDoc(t, doc, 0)
	assert.Contains(t, out, "+1.10pp")
	assert.Contains(t, out, "-0.30pp")
}

func TestStdoutEmptyDocument(t *testing.T) {
	t.Parallel()

	out := renderDoc(t, &report.Document{Title: "Nothing"}, 0)
	assert.Equal(t, "Nothing\n", out)
}

func TestStdoutRejectsNilDocument(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	r := NewStdout("stdout", &buf, 0)
	assert.Error(t, r.Report(context.Background(), nil))
}
