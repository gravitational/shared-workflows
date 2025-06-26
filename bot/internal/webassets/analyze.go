package webassets

import (
	"math"
	"sort"
)

type moduleAnalysis struct {
	newModules          []moduleChange
	increasedModules    []moduleChange
	allModules          []moduleChange
	removedModulesCount int
}

type moduleChange struct {
	name       string
	size       int64
	gzipSize   int64
	change     sizeChange
	gzipChange sizeChange
	isNew      bool
}

func analyzeModules(before, after Stats) moduleAnalysis {
	result := moduleAnalysis{}

	for moduleName, afterSize := range after.ModuleSizes {
		beforeSize, existedBefore := before.ModuleSizes[moduleName]
		afterGzipSize := after.ModuleGzipSizes[moduleName]

		var change sizeChange
		var gzipChange sizeChange
		if existedBefore {
			change = calculateSizeChange(beforeSize, afterSize)
			gzipChange = calculateSizeChange(before.ModuleGzipSizes[moduleName], afterGzipSize)
		} else {
			change = calculateSizeChange(0, afterSize)
			gzipChange = calculateSizeChange(0, afterGzipSize)
		}

		mc := moduleChange{
			name:       moduleName,
			size:       afterSize,
			gzipSize:   afterGzipSize,
			change:     change,
			gzipChange: gzipChange,
			isNew:      !existedBefore,
		}

		if !existedBefore {
			result.newModules = append(result.newModules, mc)
		} else if change.diff > significantIncreaseThreshold {
			result.increasedModules = append(result.increasedModules, mc)
		}

		result.allModules = append(result.allModules, mc)
	}

	for moduleName := range before.ModuleSizes {
		if _, exists := after.ModuleSizes[moduleName]; !exists {
			result.removedModulesCount++
		}
	}

	sort.Slice(result.newModules, func(i, j int) bool {
		return result.newModules[i].size > result.newModules[j].size
	})

	sort.Slice(result.increasedModules, func(i, j int) bool {
		return result.increasedModules[i].change.diff > result.increasedModules[j].change.diff
	})

	sort.Slice(result.allModules, func(i, j int) bool {
		return result.allModules[i].size > result.allModules[j].size
	})

	return result
}

type fileAnalysis struct {
	newFiles          []moduleChange
	removedFilesCount int
}

func analyzeFiles(before, after Stats) fileAnalysis {
	result := fileAnalysis{}

	for fileName, afterSize := range after.FileSizes {
		if _, exists := before.FileSizes[fileName]; exists {
			continue
		}

		result.newFiles = append(result.newFiles, moduleChange{
			name:     fileName,
			size:     afterSize,
			gzipSize: after.FileGzipSizes[fileName],
			isNew:    true,
			change:   calculateSizeChange(0, afterSize),
		})
	}

	for fileName := range before.FileSizes {
		if _, exists := after.FileSizes[fileName]; !exists {
			result.removedFilesCount++
		}
	}

	sort.Slice(result.newFiles, func(i, j int) bool {
		return result.newFiles[i].size > result.newFiles[j].size
	})

	return result
}

type bundleAnalysis struct {
	changedBundles []bundleChange
	newBundles     []bundleChange
}

type bundleChange struct {
	name       string
	size       int64
	gzipSize   int64
	change     sizeChange
	gzipChange sizeChange
}

func analyzeBundles(before, after Stats) bundleAnalysis {
	result := bundleAnalysis{}

	for bundleName, afterSize := range after.BundleSizes {
		beforeSize, existedBefore := before.BundleSizes[bundleName]

		change := bundleChange{
			name:     bundleName,
			size:     afterSize,
			gzipSize: after.BundleGzipSizes[bundleName],
		}

		if !existedBefore {
			change.change = calculateSizeChange(0, afterSize)
			change.gzipChange = calculateSizeChange(0, after.BundleGzipSizes[bundleName])

			result.newBundles = append(result.newBundles, change)

			continue
		}

		change.change = calculateSizeChange(beforeSize, afterSize)
		change.gzipChange = calculateSizeChange(before.BundleGzipSizes[bundleName], after.BundleGzipSizes[bundleName])

		if change.change.diff != 0 {
			result.changedBundles = append(result.changedBundles, change)
		}
	}

	sort.Slice(result.changedBundles, func(i, j int) bool {
		return math.Abs(float64(result.changedBundles[i].change.diff)) > math.Abs(float64(result.changedBundles[j].change.diff))
	})

	sort.Slice(result.newBundles, func(i, j int) bool {
		return result.newBundles[i].size > result.newBundles[j].size
	})

	return result
}
