package envloader

import (
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/loaders"
	"github.com/gravitational/trace"
)

// This is the path, relative to the git repo root, where environment
// directories can be found.
const CiDirectoryRelativePath = "environments"

func findGitRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", trace.Wrap(err, "failed to get current working directory")
	}

	// Walk upwards until a '.git' directory is found, or root is reached
	path := filepath.Clean(cwd)
	for {
		fileInfo, err := os.Stat(filepath.Join(path, ".git"))
		// If failed to stat the fs object and it exists
		if err != nil && !os.IsNotExist(err) {
			return "", trace.Wrap(err, "failed to read file information for %q", path)
		}

		// If the .git fs object was found and it is aa directory
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

func getValueFileNames(environmentPath, valueSet string) ([]string, error) {
	commonFiles, err := filepath.Glob(filepath.Join(environmentPath, "common.*"))
	if err != nil {
		return nil, trace.Wrap(err, "failed to load common values")
	}

	if valueSet == "" {
		return commonFiles, nil
	}

	valueSetFiles, err := filepath.Glob(filepath.Join(environmentPath, valueSet+".*"))
	if err != nil {
		return nil, trace.Wrap(err, "failed to load %q values", valueSet)
	}

	return append(commonFiles, valueSetFiles...), nil
}

func getValueFilePaths(environmentPath, valueSet string) ([]string, error) {
	files, err := getValueFileNames(environmentPath, valueSet)
	if err != nil {
		return nil, trace.Wrap(err, "failed to load all value file names")
	}

	for i, fileName := range files {
		files[i] = filepath.Join(environmentPath, fileName)
	}

	return files, nil
}

// Finds environment value files for the given environment and value set
func FindEnvironmentFiles(environment, valueSet string) ([]string, error) {
	repoRoot, err := findGitRepoRoot()
	if err != nil {
		return nil, trace.Wrap(err, "failed to find repo root")
	}

	environmentPath := filepath.Join(repoRoot, CiDirectoryRelativePath, environment)
	if _, err := os.Stat(environmentPath); err != nil {
		return nil, trace.Wrap(err, "failed to find environments path at %q", environmentPath)
	}

	valueFilePaths, err := getValueFilePaths(environmentPath, valueSet)
	if err != nil {
		return nil, trace.Wrap(err, "failed to load all value file paths")
	}

	return valueFilePaths, nil
}

func LoadEnvironmentValues(environment, valueSet string) (map[string]string, error) {
	valueFilePaths, err := FindEnvironmentFiles(environment, valueSet)
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
