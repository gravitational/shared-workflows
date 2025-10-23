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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTableBuilder(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		rows     [][]string
		expected []string
	}{
		{
			name:    "simple table",
			headers: []string{"Name", "Size"},
			rows: [][]string{
				{"file1.js", "100 KB"},
				{"file2.js", "200 KB"},
			},
			expected: []string{
				"| Name | Size |",
				"|---|---|",
				"| file1.js | 100 KB |",
				"| file2.js | 200 KB |",
			},
		},
		{
			name:    "table with varying column widths",
			headers: []string{"Short", "Very Long Header"},
			rows: [][]string{
				{"a", "b"},
				{"longer content", "c"},
			},
			expected: []string{
				"| Short | Very Long Header |",
				"|---|---|",
				"| a | b |",
				"| longer content | c |",
			},
		},
		{
			name:    "empty table",
			headers: []string{"Column1", "Column2"},
			rows:    [][]string{},
			expected: []string{
				"| Column1 | Column2 |",
				"|---|---|",
			},
		},
		{
			name:    "single column table",
			headers: []string{"Single"},
			rows: [][]string{
				{"row1"},
				{"row2"},
			},
			expected: []string{
				"| Single |",
				"|---|",
				"| row1 |",
				"| row2 |",
			},
		},
		{
			name:    "left aligned",
			headers: []string{":Left Aligned", "Right Aligned:"},
			rows: [][]string{
				{"left", "right"},
				{"more left", "more right"},
			},
			expected: []string{
				"| Left Aligned | Right Aligned |",
				"|:---|---:|",
				"| left | right |",
				"| more left | more right |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := newTableBuilder(tt.headers)
			for _, row := range tt.rows {
				tb.addRow(row)
			}

			result := tb.render()

			for _, expected := range tt.expected {
				require.Contains(t, result, expected)
			}

			lines := strings.Split(strings.TrimSpace(result), "\n")

			require.GreaterOrEqual(t, len(lines), 2)

			require.True(t, strings.HasPrefix(lines[0], "|"))
			require.True(t, strings.HasSuffix(lines[0], "|"))

			require.Contains(t, lines[1], "---")
		})
	}
}

func TestGenerateMarkdownTable(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		rows     [][]string
		expected []string
	}{
		{
			name:    "bundle changes table",
			headers: []string{"Bundle", "Size", "Change"},
			rows: [][]string{
				{"`main.js`", "110 KB (33 KB gz)", "+10 KB (+10.0%) ⬆️ 🟢"},
				{"`vendor.js`", "200 KB (60 KB gz)", "0 KB (0.0%) ➡️"},
			},
			expected: []string{
				"| Bundle | Size | Change |",
				"| `main.js` | 110 KB (33 KB gz) | +10 KB (+10.0%) ⬆️ 🟢 |",
				"| `vendor.js` | 200 KB (60 KB gz) | 0 KB (0.0%) ➡️ |",
			},
		},
		{
			name:    "new dependencies table",
			headers: []string{"Package", "Size"},
			rows: [][]string{
				{"`react`", "150 KB (45 KB gzipped)"},
				{"`react-dom`", "120 KB (36 KB gzipped)"},
				{"_...and 5 more_", ""},
			},
			expected: []string{
				"| Package | Size |",
				"| `react` | 150 KB (45 KB gzipped) |",
				"| `react-dom` | 120 KB (36 KB gzipped) |",
				"| _...and 5 more_ |  |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateMarkdownTable(tt.headers, tt.rows)

			for _, expected := range tt.expected {
				require.Contains(t, result, expected)
			}

			lines := strings.Split(strings.TrimSpace(result), "\n")

			for _, line := range lines {
				require.True(t, strings.HasPrefix(line, "|"))
				require.True(t, strings.HasSuffix(line, "|"))
			}

			require.Contains(t, lines[1], "---")

			headerCols := strings.Count(lines[0], "|") - 1
			for _, line := range lines {
				cols := strings.Count(line, "|") - 1
				require.Equal(t, headerCols, cols)
			}
		})
	}
}

func TestTableBuilder_EdgeCases(t *testing.T) {
	t.Run("headers with special characters", func(t *testing.T) {
		tb := newTableBuilder([]string{"Size (KB)", "Change %"})
		tb.addRow([]string{"100", "+10%"})
		result := tb.render()

		require.Contains(t, result, "| Size (KB) | Change % |")
		require.Contains(t, result, "| 100 | +10% |")
	})

	t.Run("cells with markdown formatting", func(t *testing.T) {
		tb := newTableBuilder([]string{"File", "Status"})
		tb.addRow([]string{"`main.js`", "**Updated**"})
		tb.addRow([]string{"_vendor.js_", "~~Removed~~"})
		result := tb.render()

		require.Contains(t, result, "| `main.js` | **Updated** |")
		require.Contains(t, result, "| _vendor.js_ | ~~Removed~~ |")
	})

	t.Run("empty cells", func(t *testing.T) {
		tb := newTableBuilder([]string{"Col1", "Col2", "Col3"})
		tb.addRow([]string{"a", "", "c"})
		tb.addRow([]string{"", "b", ""})
		result := tb.render()

		require.Contains(t, result, "| a |  | c |")
		require.Contains(t, result, "|  | b |  |")
	})

	t.Run("unicode and emoji", func(t *testing.T) {
		tb := newTableBuilder([]string{"Status", "Message"})
		tb.addRow([]string{"🟢", "Success"})
		tb.addRow([]string{"🔴", "Error"})
		tb.addRow([]string{"⬆️", "Increased"})
		result := tb.render()

		require.Contains(t, result, "| 🟢 | Success |")
		require.Contains(t, result, "| 🔴 | Error |")
		require.Contains(t, result, "| ⬆️ | Increased |")
	})
}
