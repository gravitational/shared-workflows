/*
 *  Copyright 2024 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package envloader

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/loaders"
	"github.com/gravitational/trace"
)

// This is the path, relative to the git repo root, where environment
// directories can be found.
const CIDirectoryRelativePath = ".environments"

// Character used to separate directories in an environment name. This must be
// consistent across multiple platforms.
const EnvironmentNameDirectorySeparator = "/"

// Glob that matches "common" files which should always be loaded. The
// values in these files have a lower "preference" than more specific
// value files.
const CommonFileGlob = "common.*"

const gitFakeLinkFileIdentifier = "gitdir: "

func findGitRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", trace.Wrap(err, "failed to get current working directory")
	}

	// Walk upwards until a '.git' directory is found, or root is reached
	path := filepath.Clean(cwd)
	for {
		gitFsObjectPath := filepath.Join(path, ".git")
		fileInfo, err := os.Lstat(gitFsObjectPath)
		// If failed to stat the fs object and it exists
		if err != nil && !os.IsNotExist(err) {
			return "", trace.Wrap(err, "failed to read file information for %q", gitFsObjectPath)
		}

		// If the .git fs object was found and it is a directory
		if err == nil {
			isCurrentPathAGitDirectory := fileInfo.IsDir()

			// Perform some rudimentary checking to see if the .git directory
			// exists elsewhere, as is the case with submodules:
			// https://git-scm.com/docs/git-init#Documentation/git-init.txt-code--separate-git-dircodeemltgit-dirgtem
			if fileInfo.Mode().IsRegular() {
				fileContents, err := os.ReadFile(gitFsObjectPath)
				if err != nil {
					return "", trace.Wrap(err, "failed to read .git file at %q", gitFsObjectPath)
				}

				isCurrentPathAGitDirectory = strings.HasPrefix(string(fileContents), gitFakeLinkFileIdentifier)
			}

			if isCurrentPathAGitDirectory {
				absPath, err := filepath.Abs(path)
				if err != nil {
					return "", trace.Wrap(err, "failed to get absolute path for git repo at %q", path)
				}

				return absPath, nil
			}
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

// Given a base path (typically the environments directory) and a relative subdirectory path
// (typically an environment name), find all common value files in every directory between
// the two, inclusively.
// NOTE: this assumes '/' is used a directory separator character. This is important so that
// the same results are produced on multiple platforms.
func findCommonFilesInPath(basePath, relativeSubdirectoryPath string) ([]string, error) {
	relativeSubdirectoryPath = filepath.ToSlash(filepath.Clean(relativeSubdirectoryPath))
	subdirectoryNames := strings.Split(relativeSubdirectoryPath, EnvironmentNameDirectorySeparator)
	// Ensure the base path is checked
	if relativeSubdirectoryPath != "." {
		subdirectoryNames = append([]string{"."}, subdirectoryNames...)
	}

	var commonFilePaths []string
	currentDirectoryPath := basePath
	for _, directoryNameToCheck := range subdirectoryNames {
		currentDirectoryPath = filepath.Join(currentDirectoryPath, directoryNameToCheck)
		fileInfo, err := os.Lstat(currentDirectoryPath)
		if err != nil {
			return nil, trace.Wrap(err, "failed to lstat %q", currentDirectoryPath)
		}
		if !fileInfo.IsDir() {
			return nil, trace.Errorf("the filesystem object at %q is not a directory", currentDirectoryPath)
		}

		foundCommonFilePaths, err := filepath.Glob(filepath.Join(currentDirectoryPath, CommonFileGlob))
		if err != nil {
			return nil, trace.Wrap(err, "failed to find common value files in directory %q", currentDirectoryPath)
		}

		commonFilePaths = append(commonFilePaths, foundCommonFilePaths...)
	}

	return commonFilePaths, nil
}

// Finds environment value files for the given environment and value set, under the given directory.
// File names will be returned in order of priority, with the lowest priority names first.
// If the environment name includes '/', it is split and each component is searched for common files.
func FindEnvironmentFilesInDirectory(environmentsDirectoryPath, environmentName string, valueSets []string) ([]string, error) {
	filePaths, err := findCommonFilesInPath(environmentsDirectoryPath, environmentName)
	if err != nil {
		return nil, trace.Wrap(err, "failed to find all common files for environment %q", environmentName)
	}

	if len(valueSets) == 0 {
		return filePaths, nil
	}

	for _, valueSet := range valueSets {
		globPath := filepath.Join(environmentsDirectoryPath, environmentName, valueSet+".*")
		valueSetFilePaths, err := filepath.Glob(globPath)
		if err != nil {
			return nil, trace.Wrap(err, "failed to find %q value files", valueSet)
		}

		if len(valueSetFilePaths) == 0 {
			return nil, trace.Errorf("failed to find any value files for %q matching %q", valueSet, globPath)
		}

		filePaths = append(filePaths, valueSetFilePaths...)
	}

	return filePaths, nil
}

// Finds environment value files for the given environment and value set, under the "environments"
// directory in the repo root. File names will be returned in order of priority, with the lowest
// priority names first.
func FindEnvironmentFiles(environment string, valueSets []string) ([]string, error) {
	repoRoot, err := findGitRepoRoot()
	if err != nil {
		return nil, trace.Wrap(err, "failed to find repo root")
	}

	environmentsPath := filepath.Join(repoRoot, CIDirectoryRelativePath)
	if _, err := os.Stat(environmentsPath); err != nil {
		return nil, trace.Wrap(err, "failed to find environments path at %q", environmentsPath)
	}

	return FindEnvironmentFilesInDirectory(environmentsPath, environment, valueSets)
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
// replaced by values from higher priority files (value set files, last provided being highest priority).
func LoadEnvironmentValues(environment string, valueSets []string) (map[string]string, error) {
	return loadEnvironmentValuesFromPaths(FindEnvironmentFiles(environment, valueSets))
}

// Finds environment value files for the given environment and value set, under the given directory,
// and loads them. Lower priority files (common files) will have values replaced by values from higher
// priority files (value set files, last provided being highest priority).
func LoadEnvironmentValuesInDirectory(directory, environment string, valueSets []string) (map[string]string, error) {
	return loadEnvironmentValuesFromPaths(FindEnvironmentFilesInDirectory(directory, environment, valueSets))
}
