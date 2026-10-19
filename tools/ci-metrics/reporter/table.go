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

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// tsvTable renders a table as tab-separated lines to permit preserving formatting
// when copying into a spreadsheet. Tabs and line breaks inside a cell are
// replaced with spaces so they do not change the table shape.
//
// Rows past maxRows are dropped, and maxRows <= 0 keeps every row. shown is
// how many rows survived, which is what a caller wants when telling the reader
// that a table was cut short.
func tsvTable(table *report.Table, maxRows int) (lines []string, shown int, truncated bool) {
	if table == nil || len(table.Columns) == 0 {
		return nil, 0, false
	}

	rows := table.Rows
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
		truncated = true
	}

	clean := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace
	lines = make([]string, 0, len(rows)+1)

	headers := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		headers[i] = clean(column.Name)
	}
	lines = append(lines, strings.Join(headers, "\t"))

	// A short row renders as empty cells rather than a ragged line, and a row
	// longer than the header is cut to the declared columns.
	for _, row := range rows {
		texts := make([]string, len(table.Columns))
		for i := range table.Columns {
			if i < len(row) {
				texts[i] = clean(row[i].Text)
			}
		}
		lines = append(lines, strings.Join(texts, "\t"))
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
