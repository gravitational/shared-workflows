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
	"testing"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/stretchr/testify/require"
)

// Ensure that the .git fs objects exist. Git won't commit these, so they
// need to be created by the tests.
func ensureGitFSObjectExist(t *testing.T, repoPath, FSObjectType string, createFunc func(gitPath string) error) {
	gitParentPath := filepath.Join(getTestDataDir(t, "repos"), repoPath)
	gitPath := filepath.Join(gitParentPath, ".git")
	_, err := os.Lstat(gitPath)
	if err == nil {
		return
	}

	if os.IsNotExist(err) {
		err = os.MkdirAll(gitParentPath, 0500)
		require.NoError(t, err, "failed to create .git parent directory at %q", gitParentPath)

		err = createFunc(gitPath)
		require.NoError(t, err, "failed to create .git %s at %q", FSObjectType, gitPath)
	} else {
		t.Fatalf("failed to check if .git %s at %q exists", FSObjectType, gitPath)
	}
}

func ensureGitFSObjectsExist(t *testing.T) {
	createGitFile := func(gitPath string) error {
		return os.Mkdir(gitPath, 0500)
	}

	ensureGitFSObjectExist(t, "basic repo", "directory", createGitFile)

	ensureGitFSObjectExist(t, "symlinked repo", "symlink", func(gitPath string) error {
		return os.Symlink("actual.git", gitPath)
	})

	ensureGitFSObjectExist(t, "git file repo", "directory", createGitFile)
	ensureGitFSObjectExist(t, filepath.Join("git file repo", "subdirectory"), "file", func(gitPath string) error {
		return os.WriteFile(gitPath, nil, 0400)
	})

	ensureGitFSObjectExist(t, "git repo with submodule", "directory", createGitFile)
	ensureGitFSObjectExist(t, filepath.Join("git repo with submodule", "submodule"), "file", func(gitPath string) error {
		// This path doesn't (currently) actually need to be created
		return os.WriteFile(gitPath, []byte("gitdir: ../.git/modules/submodule\n"), 0400)
	})

	ensureGitFSObjectExist(t, "nested repos", "directory", createGitFile)
	ensureGitFSObjectExist(t, filepath.Join("nested repos", "subdirectory"), "directory", createGitFile)
}

// This ensures that tests that change working directories don't affect each other
func getInitialCwd(t *testing.T) string {
	initialWorkingDir, err := os.Getwd()
	require.NoError(t, err, "unable to get initial working directory")
	t.Cleanup(func() {
		err := os.Chdir(initialWorkingDir)
		if err != nil {
			t.Fatalf("failed to change directory during cleanup: %#v", err)
		}
	})

	return initialWorkingDir
}

func getTestDataDir(t *testing.T, subdir ...string) string {
	cwd := getInitialCwd(t)
	return filepath.Join(append([]string{cwd, "testdata"}, subdir...)...)
}

func TestFindGitRepoRoot(t *testing.T) {
	testCases := []struct {
		desc             string
		workingDirectory string
		expectedRoot     string
		checkError       require.ErrorAssertionFunc
	}{
		{
			desc: "no repo",
			// This could fail if there is a repo in /tmp, but this is unlikely
			workingDirectory: os.TempDir(),
			checkError:       require.Error,
		},
		{
			desc:             "top of repo",
			workingDirectory: "basic repo",
			expectedRoot:     "basic repo",
		},
		{
			desc:             "starting in subdirectory",
			workingDirectory: filepath.Join("basic repo", "subdirectory 1", "subdirectory 2"),
			expectedRoot:     "basic repo",
		},
		{
			desc:             ".git file in path",
			workingDirectory: filepath.Join("git file repo", "subdirectory"),
			expectedRoot:     "git file repo",
		},
		{
			desc:             "nested repos",
			workingDirectory: filepath.Join("nested repos", "subdirectory"),
			expectedRoot:     filepath.Join("nested repos", "subdirectory"),
		},
		{
			desc:             "from submodule",
			workingDirectory: filepath.Join("git repo with submodule", "submodule"),
			expectedRoot:     filepath.Join("git repo with submodule", "submodule"),
		},
	}

	reposDirectory := getTestDataDir(t, "repos")
	ensureGitFSObjectsExist(t)

	for _, testCase := range testCases {
		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		if !filepath.IsAbs(testCase.workingDirectory) {
			testCase.workingDirectory = filepath.Join(reposDirectory, testCase.workingDirectory)
		}

		if testCase.expectedRoot != "" && !filepath.IsAbs(testCase.expectedRoot) {
			testCase.expectedRoot = filepath.Join(reposDirectory, testCase.expectedRoot)
		}

		err := os.Chdir(testCase.workingDirectory)
		require.NoError(t, err, "failed to change to test starting directory")

		actualRoot, err := findGitRepoRoot()
		require.Equal(t, testCase.expectedRoot, actualRoot, testCase.desc)
		testCase.checkError(t, err, testCase.desc)
	}
}

func TestFindEnvironmentFilesInDirectory(t *testing.T) {
	testCases := []struct {
		desc string
		// This will be prepended by the environments directory path (abs)
		environmentName string
		valueSets       []string
		// This will be prepended by the environments directory path (abs)
		expectedFileNames []string
		checkError        require.ErrorAssertionFunc
	}{
		{
			desc:            "only common file",
			environmentName: "",
			valueSets:       nil,
			expectedFileNames: []string{
				"common.abc",
			},
		},
		{
			desc:            "only environment common file",
			environmentName: "env2",
			valueSets:       nil,
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env2", "common.abc"),
			},
		},
		{
			desc:            "single file",
			environmentName: "env1",
			valueSets:       []string{"testing"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env1", "testing.abc"),
			},
		},
		{
			desc:            "missing file",
			environmentName: "env1",
			valueSets:       []string{"non existent set"},
			checkError:      require.Error,
		},
		{
			desc:            "multiple value sets in path",
			environmentName: "env2",
			valueSets:       []string{"testing1"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env2", "common.abc"),
				filepath.Join("env2", "testing1.abc"),
			},
		},
		{
			desc:            "multiple files for value set",
			environmentName: "env2",
			valueSets:       []string{"testing2"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env2", "common.abc"),
				filepath.Join("env2", "testing2.abc"),
				filepath.Join("env2", "testing2.def"),
			},
		},
		{
			desc:            "multiple file extensions",
			environmentName: "env2",
			valueSets:       []string{"testing3"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env2", "common.abc"),
				filepath.Join("env2", "testing3.abc.def"),
			},
		},
		{
			desc:            "multiple value sets",
			environmentName: "env2",
			valueSets:       []string{"testing2", "testing3"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env2", "common.abc"),
				filepath.Join("env2", "testing2.abc"),
				filepath.Join("env2", "testing2.def"),
				filepath.Join("env2", "testing3.abc.def"),
			},
		},
		{
			desc:            "no file extension",
			environmentName: "env2",
			valueSets:       []string{"testing4"},
			checkError:      require.Error,
		},
		{
			desc:            "common file exists",
			environmentName: "env3",
			valueSets:       []string{"testing"},
			expectedFileNames: []string{
				"common.abc",
				filepath.Join("env3", "common.abc"),
				filepath.Join("env3", "testing.abc"),
			},
		},
	}

	for _, testCase := range testCases {
		environmentsDirPath := getTestDataDir(t, "environments")

		for i, expectedFileName := range testCase.expectedFileNames {
			testCase.expectedFileNames[i] = filepath.Join(environmentsDirPath, expectedFileName)
		}

		if testCase.checkError == nil {
			testCase.checkError = require.NoError
		}

		actualFileNames, err := FindEnvironmentFilesInDirectory(environmentsDirPath, testCase.environmentName, testCase.valueSets)
		testCase.checkError(t, err, testCase.desc)
		require.Equal(t, testCase.expectedFileNames, actualFileNames, testCase.desc)
	}
}

func TestFindEnvironmentFiles(t *testing.T) {
	testCases := []struct {
		desc string
		// This will be prepended by the testdata directory path (abs)
		workingDir      string
		environmentName string
		valueSets       []string
		// This will be prepended by the testdata directory path (abs)
		expectedFileNames []string
	}{
		{
			desc:            "in repo",
			workingDir:      filepath.Join("repos", "basic repo", "subdirectory 1", "subdirectory 2"),
			environmentName: "env2",
			valueSets:       []string{"testing2", "testing1"},
			expectedFileNames: []string{
				// Order is important here
				filepath.Join("repos", "basic repo", ".environments", "common.def"),
				filepath.Join("repos", "basic repo", ".environments", "env2", "testing2.abc"),
				filepath.Join("repos", "basic repo", ".environments", "env2", "testing1.abc"),
			},
		},
	}

	initialWorkingDir := getInitialCwd(t)
	ensureGitFSObjectsExist(t)

	for _, testCase := range testCases {
		// Setup
		for i, expectedFileName := range testCase.expectedFileNames {
			testCase.expectedFileNames[i] = getTestDataDir(t, expectedFileName)
		}

		if testCase.workingDir != "" {
			testCase.workingDir = getTestDataDir(t, testCase.workingDir)
		}
		err := os.Chdir(testCase.workingDir)
		require.NoError(t, err, "unable to get change to test working directory")

		// Run the tests
		actualFileNames, err := FindEnvironmentFiles(testCase.environmentName, testCase.valueSets)
		require.NoError(t, err, testCase.desc)
		require.Equal(t, testCase.expectedFileNames, actualFileNames, testCase.desc)

		// Reset for the next set of tests
		err = os.Chdir(initialWorkingDir)
		require.NoError(t, err, "unable to change back to initial working directory")
	}
}

func TestLoadEnvironmentValues(t *testing.T) {
	testCases := []struct {
		desc string
		// This will be prepended by the testdata directory path (abs)
		workingDir      string
		environmentName string
		valueSets       []string
		expectedValues  map[string]values.Value
	}{
		{
			desc:            "lower priority values are overwritten",
			workingDir:      filepath.Join("repos", "basic repo", "subdirectory 1"),
			environmentName: "env1",
			valueSets:       []string{"testing2", "testing1"},
			expectedValues: map[string]values.Value{
				"topLevelCommon1": {UnderlyingValue: "top level"},
				"topLevelCommon2": {UnderlyingValue: "env level"},
				"envLevelCommon1": {UnderlyingValue: "env level"},
				"envLevelCommon2": {UnderlyingValue: "set level"},
				"setLevelCommon":  {UnderlyingValue: "testing1 level"},
				"setLevel":        {UnderlyingValue: "set level"},
			},
		},
	}

	initialWorkingDir := getInitialCwd(t)
	ensureGitFSObjectsExist(t)

	for _, testCase := range testCases {
		// Setup
		if testCase.workingDir != "" {
			testCase.workingDir = getTestDataDir(t, testCase.workingDir)
		}
		err := os.Chdir(testCase.workingDir)
		require.NoError(t, err, "unable to get change to test working directory")

		// Run the tests
		actualValues, err := LoadEnvironmentValues(testCase.environmentName, testCase.valueSets)
		require.NoError(t, err, testCase.desc)
		require.Equal(t, testCase.expectedValues, actualValues, testCase.desc)

		// Reset for the next set of tests
		err = os.Chdir(initialWorkingDir)
		require.NoError(t, err, "unable to change back to initial working directory")
	}
}
