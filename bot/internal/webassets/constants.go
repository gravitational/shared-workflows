package webassets

const (
	numberOfTopModules = 20

	newDependenciesLimit       = 10
	increasedDependenciesLimit = 5
	newFilesLimit              = 5
)

const (
	significantIncreaseThreshold = 5 * 1024 // 5 KiB

	moduleThresholdDanger  = 50 * 1024 // 50 KiB
	moduleThresholdWarning = 20 * 1024 // 20 KiB

	fileThresholdDanger      = 100 * 1024 // 100 KiB
	fileThresholdWarning     = 50 * 1024  // 50 KiB
	fileGzipThresholdDanger  = 30 * 1024  // 30 KiB
	fileGzipThresholdWarning = 15 * 1024  // 15 KiB
)
