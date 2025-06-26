/*
Copyright 2025 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webassets

import (
	"fmt"
	"strings"
)

type tableBuilder struct {
	headers []string
	rows    [][]string
}

func newTableBuilder(headers []string) *tableBuilder {
	tb := &tableBuilder{
		headers: headers,
		rows:    [][]string{},
	}

	return tb
}

func (t *tableBuilder) addRow(row []string) {
	t.rows = append(t.rows, row)
}

func (t *tableBuilder) render() string {
	var result strings.Builder

	headerRow := make([]string, len(t.headers))
	for i, h := range t.headers {
		headerRow[i] = fmt.Sprintf(" %s ", h)
	}

	result.WriteString("|")
	result.WriteString(strings.Join(headerRow, "|"))
	result.WriteString("|\n")

	separators := make([]string, len(t.headers))
	for i := range separators {
		separators[i] = "---"
	}

	result.WriteString("|")
	result.WriteString(strings.Join(separators, "|"))
	result.WriteString("|\n")

	for _, row := range t.rows {
		formattedRow := make([]string, len(row))
		for i, cell := range row {
			formattedRow[i] = fmt.Sprintf(" %s ", cell)
		}

		result.WriteString("|")
		result.WriteString(strings.Join(formattedRow, "|"))
		result.WriteString("|\n")
	}

	return result.String()
}

func generateMarkdownTable(headers []string, rows [][]string) string {
	tb := newTableBuilder(headers)

	for _, row := range rows {
		tb.addRow(row)
	}

	return tb.render()
}
