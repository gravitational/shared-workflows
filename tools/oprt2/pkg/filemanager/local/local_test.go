/*
 *  Copyright 2025 Gravitational, Inc
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

package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileManager(t *testing.T) {
	testDir := t.TempDir()

	existingDir := filepath.Join(testDir, "existing directory")
	require.NoError(t, os.Mkdir(existingDir, 0755))

	existingFile := filepath.Join(testDir, "existing file")
	require.NoError(t, os.WriteFile(existingFile, []byte("file contents"), 0644))

	existingSymlink := filepath.Join(testDir, "symlink to dir")
	require.NoError(t, os.Symlink(existingDir, existingSymlink))

	unreadableDir := filepath.Join(testDir, "unreadable dir")
	require.NoError(t, os.Mkdir(unreadableDir, 0))

	tests := []struct {
		name          string
		baseDirectory string
		errFunc       assert.ErrorAssertionFunc
	}{
		{
			name:          "existing directory",
			baseDirectory: existingDir,
		},
		{
			name:    "no path provided",
			errFunc: assert.Error,
		},
		{
			name:          "non-existant path",
			baseDirectory: "non-existant path",
			errFunc:       assert.Error,
		},
		{
			name:          "existing file",
			baseDirectory: existingFile,
			errFunc:       assert.Error,
		},
		{
			name:          "existing symlink",
			baseDirectory: existingSymlink,
			errFunc:       assert.Error,
		},
		{
			name:          "unreadable directory",
			baseDirectory: unreadableDir,
			errFunc:       assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = assert.NoError
			}

			fileManager, err := NewFileManager(tt.baseDirectory)

			tt.errFunc(t, err)
			if err == nil {
				assert.NotNil(t, fileManager)
				assert.NotNil(t, fileManager.baseDirectoryRoot)
			}
		})
	}
}

func TestName(t *testing.T) {
	testDir := t.TempDir()
	fileManager, err := NewFileManager(testDir)
	require.NoError(t, err)

	name := fileManager.Name()
	assert.Contains(t, name, "local")
	assert.Contains(t, name, testDir)
}

func TestClose(t *testing.T) {
	testDir := t.TempDir()
	fileManager, err := NewFileManager(testDir)
	require.NoError(t, err)

	err = fileManager.Close(t.Context())
	require.NoError(t, err)
	// This should error once the root is closed
	_, err = fileManager.baseDirectoryRoot.Lstat("")
	assert.Error(t, err)
}

func TestListItems(t *testing.T) {
	testDir := t.TempDir()

	topLevelItem := filepath.Join(testDir, "top-level-item")
	require.NoError(t, os.WriteFile(topLevelItem, []byte("top level item"), 0644))
	relTopLevelItem, err := filepath.Rel(testDir, topLevelItem)
	require.NoError(t, err)

	directory := filepath.Join(testDir, "directory")
	require.NoError(t, os.Mkdir(directory, 0755))

	secondLevelItem := filepath.Join(directory, "second-level-item")
	require.NoError(t, os.WriteFile(secondLevelItem, []byte("second level item"), 0644))
	relSecondLevelItem, err := filepath.Rel(testDir, secondLevelItem)
	require.NoError(t, err)

	fileManager, err := NewFileManager(testDir)
	require.NoError(t, err)

	items, err := fileManager.ListItems(t.Context())

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{relTopLevelItem, relSecondLevelItem}, items)
}

func TestGetLocalFilePath(t *testing.T) {
	testDir := t.TempDir()
	fileManager, err := NewFileManager(testDir)
	require.NoError(t, err)

	dirName := "directory"
	dir := filepath.Join(testDir, dirName)
	require.NoError(t, os.Mkdir(dir, 0755))

	fileName := "file"
	file := filepath.Join(testDir, fileName)
	require.NoError(t, os.WriteFile(file, []byte("file contents"), 0644))

	tests := []struct {
		name                  string
		item                  string
		expectedLocalFilePath string
		errFunc               assert.ErrorAssertionFunc
	}{
		{
			name:                  "existing file",
			item:                  fileName,
			expectedLocalFilePath: fileName, // Should not have dir prefix
			errFunc:               assert.NoError,
		},
		{
			name:    "non-file",
			item:    dirName,
			errFunc: assert.Error,
		},
		{
			name:    "non-existant file",
			item:    "non-existant file",
			errFunc: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localFilePath, err := fileManager.GetLocalFilePath(t.Context(), tt.item)

			tt.errFunc(t, err)
			assert.Equal(t, tt.expectedLocalFilePath, localFilePath)
		})
	}
}
