package envloader

import (
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/loaders"
	"github.com/gravitational/trace"
)

// This is the path, relative to the git repo root, where environment
// directories can be found.
const CiDirectoryRelativePath = ".environments"

func findGitRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", trace.Wrap(err, "failed to get current working directory")
	}

	// Walk upwards until a '.git' directory is found, or root is reached
	path := filepath.Clean(cwd)
	for {
		fileInfo, err := os.Lstat(filepath.Join(path, ".git"))
		// If failed to stat the fs object and it exists
		if err != nil && !os.IsNotExist(err) {
			return "", trace.Wrap(err, "failed to read file information for %q", path)
		}

		// If the .git fs object was found and it is a directory
		if err == nil && fileInfo.IsDir() {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return "", trace.Wrap(err, "failed to get absolute path for git repo at %q", path)
			}

			return absPath, nil
		}

		// If the .git fs object was found and is not a directory, or it wasn't
		// found at all, check the parent
		parent := filepath.Dir(path)
		if parent == path {
			return "", trace.Errorf("failed to find repo root")
		}

		path = parent
	}
}

// Finds environment value files for the given environment and value set, under the given directory.
// File names will be returned in order of priority, with the lowest priority names first.
func FindEnvironmentFilesInDirectory(environmentsDirectoryPath, environmentName, valueSet string) ([]string, error) {
	commonFileGlob := "common.*"
	commonFiles, err := filepath.Glob(filepath.Join(environmentsDirectoryPath, commonFileGlob))
	if err != nil {
		return nil, trace.Wrap(err, "failed to find common value files")
	}

	if environmentName == "" {
		return commonFiles, nil
	}

	environmentDirectoryPath := filepath.Join(environmentsDirectoryPath, environmentName)
	environmentCommonFiles, err := filepath.Glob(filepath.Join(environmentDirectoryPath, commonFileGlob))
	if err != nil {
		return nil, trace.Wrap(err, "failed to find common environment value files")
	}
	commonFiles = append(commonFiles, environmentCommonFiles...)

	if valueSet == "" {
		return commonFiles, nil
	}

	valueSetFiles, err := filepath.Glob(filepath.Join(environmentDirectoryPath, valueSet+".*"))
	if err != nil {
		return nil, trace.Wrap(err, "failed to find %q value files", valueSet)
	}

	matchedFiles := append(commonFiles, valueSetFiles...)
	if matchedFiles == nil {
		matchedFiles = []string{}
	}

	return matchedFiles, nil
}

// Finds environment value files for the given environment and value set, under the "environments"
// directory in the repo root. File names will be returned in order of priority, with the lowest
// priority names first.
func FindEnvironmentFiles(environment, valueSet string) ([]string, error) {
	repoRoot, err := findGitRepoRoot()
	if err != nil {
		return nil, trace.Wrap(err, "failed to find repo root")
	}

	environmentsPath := filepath.Join(repoRoot, CiDirectoryRelativePath)
	if _, err := os.Stat(environmentsPath); err != nil {
		return nil, trace.Wrap(err, "failed to find environments path at %q", environmentsPath)
	}

	return FindEnvironmentFilesInDirectory(environmentsPath, environment, valueSet)
}

func loadEnvironmentValuesFromPaths(valueFilePaths []string, err error) (map[string]string, error) {
	if err != nil {
		return nil, trace.Wrap(err, "failed to load all value file paths")
	}

	environmentValues := map[string]string{}
	for _, filePath := range valueFilePaths {
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			return nil, trace.Wrap(err, "failed to read file at %q", filePath)
		}

		fileEnvironmentValues, err := loaders.DefaultLoader.GetEnvironmentValues(fileContents)
		if err != nil {
			return nil, trace.Wrap(err, "failed to load environment values from %q", filePath)
		}

		for key, val := range fileEnvironmentValues {
			environmentValues[key] = val
		}
	}

	return environmentValues, nil
}

// Finds environment value files for the given environment and value set, under the "environments"
// directory in the repo root, and loads them. Lower priority files (common files) will have values
// replaced by values from higher priority files (value set files).
func LoadEnvironmentValues(environment, valueSet string) (map[string]string, error) {
	return loadEnvironmentValuesFromPaths(FindEnvironmentFiles(environment, valueSet))
}

// Finds environment value files for the given environment and value set, under the given directory,
// and loads them. Lower priority files (common files) will have values replaced by values from higher
// priority files (value set files).
func LoadEnvironmentValuesInDirectory(directory, environment, valueSet string) (map[string]string, error) {
	return loadEnvironmentValuesFromPaths(FindEnvironmentFilesInDirectory(directory, environment, valueSet))
}
