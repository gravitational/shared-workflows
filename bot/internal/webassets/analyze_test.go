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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeModules(t *testing.T) {
	tests := []struct {
		name                     string
		before                   Stats
		after                    Stats
		expectedNewModules       []string
		expectedIncreasedModules []string
		expectedRemovedCount     int
	}{
		{
			name: "new modules detected",
			before: Stats{
				ModuleSizes: map[string]int64{
					"react": 150000,
				},
				ModuleGzipSizes: map[string]int64{
					"react": 45000,
				},
			},
			after: Stats{
				ModuleSizes: map[string]int64{
					"react":     150000,
					"react-dom": 120000,
					"lodash":    80000,
				},
				ModuleGzipSizes: map[string]int64{
					"react":     45000,
					"react-dom": 36000,
					"lodash":    24000,
				},
			},
			expectedNewModules:       []string{"react-dom", "lodash"},
			expectedIncreasedModules: []string{},
			expectedRemovedCount:     0,
		},
		{
			name: "increased modules detected",
			before: Stats{
				ModuleSizes: map[string]int64{
					"react":     150000,
					"react-dom": 100000,
				},
				ModuleGzipSizes: map[string]int64{
					"react":     45000,
					"react-dom": 30000,
				},
			},
			after: Stats{
				ModuleSizes: map[string]int64{
					"react":     150000,
					"react-dom": 110000, // 10KB increase, over threshold
				},
				ModuleGzipSizes: map[string]int64{
					"react":     45000,
					"react-dom": 33000,
				},
			},
			expectedNewModules:       []string{},
			expectedIncreasedModules: []string{"react-dom"},
			expectedRemovedCount:     0,
		},
		{
			name: "removed modules detected",
			before: Stats{
				ModuleSizes: map[string]int64{
					"react":     150000,
					"react-dom": 120000,
					"lodash":    80000,
				},
				ModuleGzipSizes: map[string]int64{
					"react":     45000,
					"react-dom": 36000,
					"lodash":    24000,
				},
			},
			after: Stats{
				ModuleSizes: map[string]int64{
					"react": 150000,
				},
				ModuleGzipSizes: map[string]int64{
					"react": 45000,
				},
			},
			expectedNewModules:       []string{},
			expectedIncreasedModules: []string{},
			expectedRemovedCount:     2,
		},
		{
			name: "small increases ignored",
			before: Stats{
				ModuleSizes: map[string]int64{
					"react": 150000,
				},
				ModuleGzipSizes: map[string]int64{
					"react": 45000,
				},
			},
			after: Stats{
				ModuleSizes: map[string]int64{
					"react": 151000, // Only 1KB increase, below threshold
				},
				ModuleGzipSizes: map[string]int64{
					"react": 45300,
				},
			},
			expectedNewModules:       []string{},
			expectedIncreasedModules: []string{},
			expectedRemovedCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzeModules(tt.before, tt.after)

			newModuleNames := make([]string, len(result.newModules))
			for i, m := range result.newModules {
				newModuleNames[i] = m.name
			}

			require.ElementsMatch(t, tt.expectedNewModules, newModuleNames)

			increasedModuleNames := make([]string, len(result.increasedModules))
			for i, m := range result.increasedModules {
				increasedModuleNames[i] = m.name
			}

			require.ElementsMatch(t, tt.expectedIncreasedModules, increasedModuleNames)

			require.Equal(t, tt.expectedRemovedCount, result.removedModulesCount)

			for i := 1; i < len(result.allModules); i++ {
				require.GreaterOrEqual(t, result.allModules[i-1].size, result.allModules[i].size)
			}
		})
	}
}

func TestAnalyzeFiles(t *testing.T) {
	tests := []struct {
		name                 string
		before               Stats
		after                Stats
		expectedNewFiles     []string
		expectedRemovedCount int
	}{
		{
			name: "new files detected",
			before: Stats{
				FileSizes: map[string]int64{
					"app.js": 50000,
				},
				FileGzipSizes: map[string]int64{
					"app.js": 15000,
				},
			},
			after: Stats{
				FileSizes: map[string]int64{
					"app.js":     50000,
					"vendor.js":  100000,
					"styles.css": 25000,
				},
				FileGzipSizes: map[string]int64{
					"app.js":     15000,
					"vendor.js":  30000,
					"styles.css": 8000,
				},
			},
			expectedNewFiles:     []string{"vendor.js", "styles.css"},
			expectedRemovedCount: 0,
		},
		{
			name: "removed files detected",
			before: Stats{
				FileSizes: map[string]int64{
					"app.js":     50000,
					"vendor.js":  100000,
					"styles.css": 25000,
				},
				FileGzipSizes: map[string]int64{
					"app.js":     15000,
					"vendor.js":  30000,
					"styles.css": 8000,
				},
			},
			after: Stats{
				FileSizes: map[string]int64{
					"app.js": 50000,
				},
				FileGzipSizes: map[string]int64{
					"app.js": 15000,
				},
			},
			expectedNewFiles:     []string{},
			expectedRemovedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzeFiles(tt.before, tt.after)

			newFileNames := make([]string, len(result.newFiles))
			for i, f := range result.newFiles {
				newFileNames[i] = f.name
			}

			require.ElementsMatch(t, tt.expectedNewFiles, newFileNames)

			require.Equal(t, tt.expectedRemovedCount, result.removedFilesCount)

			for i := 1; i < len(result.newFiles); i++ {
				require.GreaterOrEqual(t, result.newFiles[i-1].size, result.newFiles[i].size)
			}
		})
	}
}

func TestAnalyzeBundles(t *testing.T) {
	tests := []struct {
		name                   string
		before                 Stats
		after                  Stats
		expectedNewBundles     []string
		expectedChangedBundles []string
	}{
		{
			name: "new bundles detected",
			before: Stats{
				BundleSizes: map[string]int64{
					"main.js": 100000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js": 30000,
				},
			},
			after: Stats{
				BundleSizes: map[string]int64{
					"main.js":   100000,
					"vendor.js": 200000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js":   30000,
					"vendor.js": 60000,
				},
			},
			expectedNewBundles:     []string{"vendor.js"},
			expectedChangedBundles: []string{},
		},
		{
			name: "changed bundles detected",
			before: Stats{
				BundleSizes: map[string]int64{
					"main.js":   100000,
					"vendor.js": 200000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js":   30000,
					"vendor.js": 60000,
				},
			},
			after: Stats{
				BundleSizes: map[string]int64{
					"main.js":   110000, // increased
					"vendor.js": 190000, // decreased
				},
				BundleGzipSizes: map[string]int64{
					"main.js":   33000,
					"vendor.js": 57000,
				},
			},
			expectedNewBundles:     []string{},
			expectedChangedBundles: []string{"main.js", "vendor.js"},
		},
		{
			name: "unchanged bundles ignored",
			before: Stats{
				BundleSizes: map[string]int64{
					"main.js": 100000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js": 30000,
				},
			},
			after: Stats{
				BundleSizes: map[string]int64{
					"main.js": 100000,
				},
				BundleGzipSizes: map[string]int64{
					"main.js": 30000,
				},
			},
			expectedNewBundles:     []string{},
			expectedChangedBundles: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzeBundles(tt.before, tt.after)

			newBundleNames := make([]string, len(result.newBundles))
			for i, b := range result.newBundles {
				newBundleNames[i] = b.name
			}

			require.ElementsMatch(t, tt.expectedNewBundles, newBundleNames)

			changedBundleNames := make([]string, len(result.changedBundles))
			for i, b := range result.changedBundles {
				changedBundleNames[i] = b.name
			}

			require.ElementsMatch(t, tt.expectedChangedBundles, changedBundleNames)

			for _, b := range result.newBundles {
				require.True(t, b.change.isNew)
			}
		})
	}
}
