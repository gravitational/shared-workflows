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
)

type comparison struct {
	bundleAnalysis
	fileAnalysis
	moduleAnalysis
	totalChange     sizeChange
	totalGzipChange sizeChange
}

type sizeChange struct {
	before        int64
	after         int64
	diff          int64
	percentChange float64
	isNew         bool
}

func Compare(before, after Stats) (string, error) {
	c := comparison{
		bundleAnalysis:  analyzeBundles(before, after),
		fileAnalysis:    analyzeFiles(before, after),
		moduleAnalysis:  analyzeModules(before, after),
		totalChange:     calculateSizeChange(before.TotalSize, after.TotalSize),
		totalGzipChange: calculateSizeChange(before.TotalGzipSize, after.TotalGzipSize),
	}

	return c.String(), nil
}

// String creates a formatted string representation of the comparison report.
func (c comparison) String() string {
	var b strings.Builder

	renderHeader(&b)
	renderSummary(&b, c.totalChange, c.totalGzipChange)

	renderDetailsStart(&b, "Full Report")

	if len(c.newBundles) > 0 {
		renderNewBundles(&b, c.newBundles)
	}

	if len(c.changedBundles) > 0 {
		renderBundleChanges(&b, c.changedBundles)
	}

	if len(c.newModules) > 0 {
		renderNewDependencies(&b, c.newModules)
	}

	if len(c.increasedModules) > 0 {
		renderIncreasedDependencies(&b, c.increasedModules)
	}

	if len(c.newFiles) > 0 {
		renderNewFiles(&b, c.newFiles)
	}

	renderTopDependencies(&b, c.allModules)

	renderDetailsEnd(&b)

	return b.String()
}

func calculateSizeChange(before, after int64) sizeChange {
	var percentChange float64
	var isNew bool

	diff := after - before

	if before == 0 {
		if after == 0 {
			percentChange = 0
		} else {
			percentChange = 100
			isNew = true
		}
	} else {
		percentChange = float64(diff) / float64(before) * 100
	}

	return sizeChange{
		before:        before,
		after:         after,
		diff:          diff,
		percentChange: percentChange,
		isNew:         isNew,
	}
}
