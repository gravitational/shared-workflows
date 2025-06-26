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

func TestCalculateSizeChange(t *testing.T) {
	tests := []struct {
		name            string
		before          int64
		after           int64
		expectedDiff    int64
		expectedPercent float64
		expectedIsNew   bool
	}{
		{
			name:            "increase",
			before:          100000,
			after:           150000,
			expectedDiff:    50000,
			expectedPercent: 50.0,
			expectedIsNew:   false,
		},
		{
			name:            "decrease",
			before:          150000,
			after:           100000,
			expectedDiff:    -50000,
			expectedPercent: -33.33333333333333,
			expectedIsNew:   false,
		},
		{
			name:            "no change",
			before:          100000,
			after:           100000,
			expectedDiff:    0,
			expectedPercent: 0.0,
			expectedIsNew:   false,
		},
		{
			name:            "new file",
			before:          0,
			after:           100000,
			expectedDiff:    100000,
			expectedPercent: 100.0,
			expectedIsNew:   true,
		},
		{
			name:            "removed file",
			before:          100000,
			after:           0,
			expectedDiff:    -100000,
			expectedPercent: -100.0,
			expectedIsNew:   false,
		},
		{
			name:            "both zero",
			before:          0,
			after:           0,
			expectedDiff:    0,
			expectedPercent: 0.0,
			expectedIsNew:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateSizeChange(tt.before, tt.after)

			require.Equal(t, tt.before, result.before)
			require.Equal(t, tt.after, result.after)
			require.Equal(t, tt.expectedDiff, result.diff)
			require.Equal(t, tt.expectedPercent, result.percentChange)
			require.Equal(t, tt.expectedIsNew, result.isNew)
		})
	}
}

func TestCompare(t *testing.T) {
	before := Stats{
		BundleSizes: map[string]int64{
			"main.js":   100000,
			"vendor.js": 200000,
		},
		BundleGzipSizes: map[string]int64{
			"main.js":   30000,
			"vendor.js": 60000,
		},
		FileSizes: map[string]int64{
			"app.js": 50000,
		},
		FileGzipSizes: map[string]int64{
			"app.js": 15000,
		},
		ModuleSizes: map[string]int64{
			"react":     150000,
			"react-dom": 120000,
		},
		ModuleGzipSizes: map[string]int64{
			"react":     45000,
			"react-dom": 36000,
		},
		TotalSize:     300000,
		TotalGzipSize: 90000,
	}

	after := Stats{
		BundleSizes: map[string]int64{
			"main.js":   110000, // increased
			"vendor.js": 200000, // unchanged
			"worker.js": 50000,  // new
		},
		BundleGzipSizes: map[string]int64{
			"main.js":   33000,
			"vendor.js": 60000,
			"worker.js": 15000,
		},
		FileSizes: map[string]int64{
			"app.js":     50000,
			"vendor.js":  100000, // new
			"styles.css": 25000,  // new
		},
		FileGzipSizes: map[string]int64{
			"app.js":     15000,
			"vendor.js":  30000,
			"styles.css": 8000,
		},
		ModuleSizes: map[string]int64{
			"react":     150000,
			"react-dom": 130000, // increased by 10KB
			"lodash":    80000,  // new
		},
		ModuleGzipSizes: map[string]int64{
			"react":     45000,
			"react-dom": 39000,
			"lodash":    24000,
		},
		TotalSize:     350000,
		TotalGzipSize: 105000,
	}

	result, err := Compare(before, after)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	require.Contains(t, result, "# 📦 Bundle Size Report")
	require.Contains(t, result, "### Summary")
	require.Contains(t, result, "**Total Size:**")
	require.Contains(t, result, "**Gzipped:**")

	require.Contains(t, result, "### 🆕 New Bundles")
	require.Contains(t, result, "worker.js")

	require.Contains(t, result, "### 📊 Bundle Changes")
	require.Contains(t, result, "main.js")

	require.Contains(t, result, "### 🆕 New Dependencies")
	require.Contains(t, result, "lodash")

	require.Contains(t, result, "### 📈 Increased Dependencies")
	require.Contains(t, result, "react-dom")

	require.Contains(t, result, "### 📄 New Files")
	require.Contains(t, result, "vendor.js")
	require.Contains(t, result, "styles.css")

	require.Contains(t, result, "Full Report")
	require.Contains(t, result, "<details>")
	require.Contains(t, result, "</details>")
}

func TestComparisonString_EmptyChanges(t *testing.T) {
	before := Stats{
		BundleSizes: map[string]int64{
			"main.js": 100000,
		},
		BundleGzipSizes: map[string]int64{
			"main.js": 30000,
		},
		TotalSize:     100000,
		TotalGzipSize: 30000,
	}

	after := Stats{
		BundleSizes: map[string]int64{
			"main.js": 100000,
		},
		BundleGzipSizes: map[string]int64{
			"main.js": 30000,
		},
		TotalSize:     100000,
		TotalGzipSize: 30000,
	}

	result, err := Compare(before, after)

	require.NoError(t, err)
	require.NotEmpty(t, result)

	require.Contains(t, result, "# 📦 Bundle Size Report")
	require.Contains(t, result, "### Summary")

	require.NotContains(t, result, "### 🆕 New Bundles")
	require.NotContains(t, result, "### 📊 Bundle Changes")
	require.NotContains(t, result, "### 🆕 New Dependencies")
	require.NotContains(t, result, "### 📈 Increased Dependencies")
}

func TestComparison_LargeLists(t *testing.T) {
	before := Stats{
		ModuleSizes:     map[string]int64{},
		ModuleGzipSizes: map[string]int64{},
		FileSizes:       map[string]int64{},
		FileGzipSizes:   map[string]int64{},
		BundleSizes:     map[string]int64{},
		BundleGzipSizes: map[string]int64{},
	}

	after := Stats{
		ModuleSizes:     map[string]int64{},
		ModuleGzipSizes: map[string]int64{},
		FileSizes:       map[string]int64{},
		FileGzipSizes:   map[string]int64{},
		BundleSizes:     map[string]int64{},
		BundleGzipSizes: map[string]int64{},
	}

	for i := 0; i < 15; i++ {
		moduleName := strings.Repeat("a", i+1)

		after.ModuleSizes[moduleName] = int64(100000 + i*1000)
		after.ModuleGzipSizes[moduleName] = int64(30000 + i*300)
	}

	for i := 0; i < 10; i++ {
		fileName := strings.Repeat("b", i+1) + ".js"

		after.FileSizes[fileName] = int64(50000 + i*1000)
		after.FileGzipSizes[fileName] = int64(15000 + i*300)
	}

	result, err := Compare(before, after)

	require.NoError(t, err)

	require.Contains(t, result, "...and")
	require.Contains(t, result, "more")

	newDepsStart := strings.Index(result, "### 🆕 New Dependencies")
	require.NotEqual(t, -1, newDepsStart)

	restOfResult := result[newDepsStart:]
	nextSectionIndex := strings.Index(restOfResult[1:], "###")
	detailsIndex := strings.Index(restOfResult, "<details>")

	var sectionEnd int
	if nextSectionIndex != -1 && (detailsIndex == -1 || nextSectionIndex < detailsIndex) {
		sectionEnd = nextSectionIndex + 1
	} else if detailsIndex != -1 {
		sectionEnd = detailsIndex
	} else {
		sectionEnd = len(restOfResult)
	}

	newDepsSection := restOfResult[:sectionEnd]

	moduleCount := 0
	for i := 1; i <= 15; i++ {
		moduleName := strings.Repeat("a", i)
		if strings.Contains(newDepsSection, "`"+moduleName+"`") {
			moduleCount++
		}
	}

	require.Equal(t, newDependenciesLimit, moduleCount)
	require.Contains(t, newDepsSection, "and 5 more")

	newFilesStart := strings.Index(result, "### 📄 New Files")

	require.NotEqual(t, -1, newFilesStart)

	restOfResult = result[newFilesStart:]
	nextSectionIndex = strings.Index(restOfResult[1:], "###")
	detailsIndex = strings.Index(restOfResult, "<details>")

	if nextSectionIndex != -1 && (detailsIndex == -1 || nextSectionIndex < detailsIndex) {
		sectionEnd = nextSectionIndex + 1
	} else if detailsIndex != -1 {
		sectionEnd = detailsIndex
	} else {
		sectionEnd = len(restOfResult)
	}

	newFilesSection := restOfResult[:sectionEnd]

	fileCount := 0
	for i := 1; i <= 10; i++ {
		fileName := strings.Repeat("b", i) + ".js"

		if strings.Contains(newFilesSection, "`"+fileName+"`") {
			fileCount++
		}
	}

	require.Equal(t, newFilesLimit, fileCount)
	require.Contains(t, newFilesSection, "and 5 more")
}
