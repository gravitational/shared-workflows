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
	"strings"
	"unicode/utf8"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// textTable renders a table as fixed-width lines, the header first and one
// line per row, with every column padded to its widest cell so that digits
// line up by place value. The lines carry no leading indent and no trailing
// newline; the caller decides how to frame them.
//
// Rows past maxRows are dropped, and maxRows <= 0 keeps every row. shown is
// how many rows survived, which is what a caller wants when telling the reader
// that a table was cut short.
//
// This is the plain text reporter's rendering. Slack has a markdown block that
// renders a real table, so it uses [markdownTable] instead.
func textTable(table *report.Table, maxRows int) (lines []string, shown int, truncated bool) {
	const columnGutter = "  "

	if table == nil || len(table.Columns) == 0 {
		return nil, 0, false
	}

	rows := table.Rows
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		truncated = true
	}

	// Widths come from the rows actually rendered, so dropping rows never
	// leaves a column padded for a value the reader cannot see.
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

	format := func(texts []string) string {
		var b strings.Builder
		for i, text := range texts {
			if i > 0 {
				b.WriteString(columnGutter)
			}
			pad := max(widths[i]-utf8.RuneCountInString(text), 0)
			switch {
			case table.Columns[i].Align == report.AlignRight:
				b.WriteString(strings.Repeat(" ", pad))
				b.WriteString(text)
			case i == len(texts)-1:
				// The final column needs no trailing padding
				b.WriteString(text)
			default:
				b.WriteString(text)
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		return b.String()
	}

	lines = make([]string, 0, len(rows)+1)

	headers := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		headers[i] = c.Name
	}
	lines = append(lines, format(headers))

	// A short row renders as empty cells rather than a ragged line, and a row
	// longer than the header is cut to the declared columns.
	for _, row := range rows {
		texts := make([]string, len(table.Columns))
		for i := range table.Columns {
			if i < len(row) {
				texts[i] = row[i].Text
			}
		}
		lines = append(lines, format(texts))
	}

	return lines, len(rows), truncated
}

// markdownCellEscaper neutralises the characters that would end a table cell
// early or be read as emphasis. Go test names carry underscores, brackets and
// generic type parameters.
var markdownCellEscaper = strings.NewReplacer(
	`\`, `\\`,
	"|", `\|`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"[", `\[`,
	"]", `\]`,
	"~", `\~`,
	"\n", " ",
	"\r", " ",
)

// markdownTable renders a table in GitHub-flavored markdown, which Slack's
// markdown block turns into a real table. Columns carry their alignment in the
// delimiter row, so nothing has to be padded to a fixed width.
func markdownTable(table *report.Table, maxRows int) (md string, shown int, truncated bool) {
	if table == nil || len(table.Columns) == 0 {
		return "", 0, false
	}

	rows := table.Rows
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		truncated = true
	}

	var b strings.Builder

	writeRow := func(texts []string) {
		b.WriteString("|")
		for _, text := range texts {
			b.WriteString(" ")
			b.WriteString(markdownCellEscaper.Replace(text))
			b.WriteString(" |")
		}
	}

	headers := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		headers[i] = c.Name
	}
	writeRow(headers)

	b.WriteString("\n|")
	for _, c := range table.Columns {
		if c.Align == report.AlignRight {
			b.WriteString(" ---: |")
			continue
		}
		b.WriteString(" :--- |")
	}

	// A short row gets empty cells; a long one is cut to the declared columns.
	for _, row := range rows {
		texts := make([]string, len(table.Columns))
		for i := range table.Columns {
			if i < len(row) {
				texts[i] = row[i].Text
			}
		}
		b.WriteString("\n")
		writeRow(texts)
	}

	return b.String(), len(rows), truncated
}
