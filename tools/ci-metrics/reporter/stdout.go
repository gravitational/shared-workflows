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
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

var _ Reporter = (*Stdout)(nil)

const TypeStdout = "stdout"

// Stdout renders a document as plain text.
type Stdout struct {
	name string
	out  io.Writer
	// maxRows caps the rows printed per table. <=0 means no limit.
	maxRows int
}

// NewStdout creates a new instance of [Stdout].
func NewStdout(name string, out io.Writer, maxRows int) *Stdout {
	return &Stdout{name: name, out: out, maxRows: maxRows}
}

// Name returns the configured name of this reporter.
func (s *Stdout) Name() string {
	return s.name
}

// Close implements [Reporter].
func (s *Stdout) Close() error {
	return nil
}

// Report writes the document as text.
func (s *Stdout) Report(_ context.Context, doc *report.Document) error {
	if doc == nil {
		return trace.BadParameter("document is required")
	}

	var b strings.Builder

	b.WriteString(doc.Title)
	b.WriteByte('\n')
	if doc.Subtitle != "" {
		b.WriteString(doc.Subtitle)
		b.WriteByte('\n')
	}
	if doc.Headline != "" {
		b.WriteByte('\n')
		b.WriteString(doc.Headline)
		b.WriteByte('\n')
	}

	for _, section := range doc.Sections {
		b.WriteByte('\n')
		if section.Heading != "" {
			b.WriteString(section.Heading)
			b.WriteByte('\n')
		}
		if section.Text != "" {
			b.WriteString(indent(section.Text))
			b.WriteByte('\n')
		}
		writeMetrics(&b, section.Metrics)
		s.writeTable(&b, section.Table)
		for _, note := range section.Notes {
			b.WriteString("  - ")
			b.WriteString(note)
			b.WriteByte('\n')
		}
	}

	if scanned := doc.Meta.DataScannedBytes; scanned > 0 {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "%s scanned across %d query/queries\n",
			report.Bytes(scanned), len(doc.Meta.QueryExecutionIDs))
	}

	if _, err := io.WriteString(s.out, b.String()); err != nil {
		return trace.Wrap(err, "writing report %s", doc.ID)
	}
	return nil
}

// writeMetrics prints name/value pairs in a left-aligned block.
func writeMetrics(b *strings.Builder, metrics []report.Metric) {
	if len(metrics) == 0 {
		return
	}

	var width int
	for _, m := range metrics {
		width = max(width, utf8.RuneCountInString(m.Name))
	}

	for _, m := range metrics {
		fmt.Fprintf(b, "  %-*s  %s", width, m.Name, m.Value)
		if m.Delta != nil {
			b.WriteString("  ")
			b.WriteString(arrow(m.Delta.Up))
			b.WriteString(m.Delta.Text)
		}
		b.WriteByte('\n')
	}
}

// writeTable prints a table with per-column alignment.
func (s *Stdout) writeTable(b *strings.Builder, table *report.Table) {
	if table == nil || len(table.Columns) == 0 {
		return
	}

	rows := table.Rows
	truncated := false
	if s.maxRows > 0 && len(rows) > s.maxRows {
		rows = rows[:s.maxRows]
		truncated = true
	}

	widths := make([]int, len(table.Columns))
	for i, c := range table.Columns {
		widths[i] = utf8.RuneCountInString(c.Name)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(cell.Text))
		}
	}

	writeCells := func(texts []string) {
		b.WriteString("  ")
		for i, text := range texts {
			if i > 0 {
				b.WriteString("  ")
			}
			pad := widths[i] - utf8.RuneCountInString(text)
			if pad < 0 {
				pad = 0
			}
			// The final column needs no trailing padding, which would only
			// show up as trailing whitespace in a golden file.
			switch {
			case table.Columns[i].Align == report.AlignRight:
				b.WriteString(strings.Repeat(" ", pad))
				b.WriteString(text)
			case i == len(texts)-1:
				b.WriteString(text)
			default:
				b.WriteString(text)
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteByte('\n')
	}

	headers := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		headers[i] = c.Name
	}
	writeCells(headers)

	for _, row := range rows {
		texts := make([]string, len(table.Columns))
		for i := range table.Columns {
			if i < len(row) {
				texts[i] = row[i].Text
			}
		}
		writeCells(texts)
	}

	if truncated {
		fmt.Fprintf(b, "  ... showing %d of %d\n", len(rows), table.TotalRows)
	}
}

// indent prefixes every line of s with two spaces.
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = "  " + l
	}
	return strings.Join(lines, "\n")
}

// arrow renders the direction of a delta.
func arrow(up bool) string {
	if up {
		return "+"
	}
	return "-"
}
