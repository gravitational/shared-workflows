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

func TestIsBotComment(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected bool
	}{
		{
			name:     "valid bot comment",
			comment:  "# 📦 Bundle Size Report\n\nSome content",
			expected: true,
		},
		{
			name:     "valid bot comment without content",
			comment:  "# 📦 Bundle Size Report",
			expected: true,
		},
		{
			name:     "not a bot comment",
			comment:  "This is a regular comment",
			expected: false,
		},
		{
			name:     "similar but not exact header",
			comment:  "## 📦 Bundle Size Report",
			expected: false,
		},
		{
			name:     "header in middle of comment",
			comment:  "Some text\n# 📦 Bundle Size Report",
			expected: false,
		},
		{
			name:     "empty comment",
			comment:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBotComment(tt.comment)

			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
		{
			name:     "bytes",
			bytes:    500,
			expected: "500.00 B",
		},
		{
			name:     "kilobytes",
			bytes:    1536,
			expected: "1.50 KB",
		},
		{
			name:     "megabytes",
			bytes:    1048576,
			expected: "1.00 MB",
		},
		{
			name:     "gigabytes",
			bytes:    1073741824,
			expected: "1.00 GB",
		},
		{
			name:     "large kilobytes",
			bytes:    102400,
			expected: "100.00 KB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)

			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetChangeColor(t *testing.T) {
	tests := []struct {
		name          string
		diff          int64
		thresholdType string
		gzipped       bool
		expected      string
	}{
		{
			name:          "negative change (improvement)",
			diff:          -10000,
			thresholdType: "file",
			gzipped:       false,
			expected:      "🟢",
		},
		{
			name:          "small increase - module",
			diff:          10000,
			thresholdType: "module",
			gzipped:       false,
			expected:      "🟢",
		},
		{
			name:          "warning threshold - module",
			diff:          25000,
			thresholdType: "module",
			gzipped:       false,
			expected:      "🟡",
		},
		{
			name:          "danger threshold - module",
			diff:          60000,
			thresholdType: "module",
			gzipped:       false,
			expected:      "🔴",
		},
		{
			name:          "warning threshold - file",
			diff:          60000,
			thresholdType: "file",
			gzipped:       false,
			expected:      "🟡",
		},
		{
			name:          "danger threshold - file",
			diff:          110000,
			thresholdType: "file",
			gzipped:       false,
			expected:      "🔴",
		},
		{
			name:          "warning threshold - gzipped file",
			diff:          20000,
			thresholdType: "file",
			gzipped:       true,
			expected:      "🟡",
		},
		{
			name:          "danger threshold - gzipped file",
			diff:          35000,
			thresholdType: "file",
			gzipped:       true,
			expected:      "🔴",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChangeColor(tt.diff, tt.thresholdType, tt.gzipped)

			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetChangeIndicator(t *testing.T) {
	tests := []struct {
		name          string
		change        sizeChange
		thresholdType string
		gzipped       bool
		expected      string
	}{
		{
			name: "no change",
			change: sizeChange{
				diff: 0,
			},
			thresholdType: "file",
			gzipped:       false,
			expected:      "➡️",
		},
		{
			name: "small increase",
			change: sizeChange{
				diff: 1000,
			},
			thresholdType: "file",
			gzipped:       false,
			expected:      "⬆️ 🟢",
		},
		{
			name: "significant increase - green",
			change: sizeChange{
				diff: 10000,
			},
			thresholdType: "module",
			gzipped:       false,
			expected:      "⬆️ 🟢",
		},
		{
			name: "significant increase - yellow",
			change: sizeChange{
				diff: 25000,
			},
			thresholdType: "module",
			gzipped:       false,
			expected:      "⬆️ 🟡",
		},
		{
			name: "decrease",
			change: sizeChange{
				diff: -10000,
			},
			thresholdType: "file",
			gzipped:       false,
			expected:      "⬇️ 🟢",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getChangeIndicator(tt.change, tt.thresholdType, tt.gzipped)

			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSizeChange(t *testing.T) {
	tests := []struct {
		name          string
		change        sizeChange
		thresholdType string
		gzipped       bool
		expected      string
	}{
		{
			name:     "new item",
			change:   calculateSizeChange(0, 100000),
			expected: "🆕 97.66 KB (+97.66 KB)",
		},
		{
			name:          "increase",
			change:        calculateSizeChange(100000, 150000),
			thresholdType: "file",
			gzipped:       false,
			expected:      "146.48 KB (+48.83 KB, +50.0%) ⬆️ 🟢",
		},
		{
			name:          "decrease",
			change:        calculateSizeChange(150000, 100000),
			thresholdType: "file",
			gzipped:       false,
			expected:      "97.66 KB (-48.83 KB, -33.3%) ⬇️ 🟢",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSizeChange(tt.change, tt.thresholdType, tt.gzipped)

			require.Equal(t, tt.expected, result)

			if tt.change.isNew {
				require.Contains(t, result, "🆕")
			}

			require.Contains(t, result, "KB")

			if !tt.change.isNew && tt.change.diff != 0 {
				require.Contains(t, result, "%")
			}
		})
	}
}

func TestRenderSummary(t *testing.T) {
	var b strings.Builder

	totalChange := sizeChange{
		before:        300000,
		after:         350000,
		diff:          50000,
		percentChange: 16.7,
	}
	totalGzipChange := sizeChange{
		before:        90000,
		after:         105000,
		diff:          15000,
		percentChange: 16.7,
	}

	renderSummary(&b, totalChange, totalGzipChange)
	result := b.String()

	require.Contains(t, result, "### Summary")
	require.Contains(t, result, "**Total Size:** 341.80 KB (+48.83 KB, +16.7%) ⬆️ 🟢")
	require.Contains(t, result, "**Gzipped:** 102.54 KB (+14.65 KB, +16.7%) ⬆️ 🟢")
}

func TestRenderTopDependencies(t *testing.T) {
	var b strings.Builder

	modules := []moduleChange{
		{
			name:     "react",
			size:     500000,
			gzipSize: 150000,
		},
		{
			name:     "lodash",
			size:     300000,
			gzipSize: 90000,
		},
		{
			name:     "vue",
			size:     250000,
			gzipSize: 75000,
		},
		{
			name:     "moment",
			size:     200000,
			gzipSize: 60000,
		},
		{
			name:     "axios",
			size:     150000,
			gzipSize: 45000,
		},
	}

	renderTopDependencies(&b, modules)
	result := b.String()

	require.Contains(t, result, "### Top 20 Dependencies")

	require.Contains(t, result, "`react`")
	require.Contains(t, result, "`lodash`")
	require.Contains(t, result, "`vue`")
	require.Contains(t, result, "`moment`")
	require.Contains(t, result, "`axios`")

	require.Contains(t, result, "488.28 KB (146.48 KB gzipped)")
	require.Contains(t, result, "292.97 KB (87.89 KB gzipped)")
}
