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
	"context"
	"fmt"
	"io/fs"
	"os"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
)

// FileManager is a [filemanager.FileManager] that retrieves files from the local disk.
type FileManager struct {
	baseDirectory     string
	baseDirectoryRoot *os.Root
}

var _ filemanager.FileManager = &FileManager{}

func NewFileManager(baseDirectory string) (*FileManager, error) {
	// Fail if the directory does not exist or is not a directory
	dir, err := os.Lstat(baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to verify that directory %q exists: %w", baseDirectory, err)
	}

	if !dir.IsDir() {
		return nil, fmt.Errorf("provided directory path %q is not a directory", baseDirectory)
	}

	root, err := os.OpenRoot(baseDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to open base directory %q: %w", baseDirectory, err)
	}

	return &FileManager{
		baseDirectory:     baseDirectory,
		baseDirectoryRoot: root,
	}, nil
}

func (lfm *FileManager) Name() string {
	return "local@" + lfm.baseDirectory
}

func (lfm *FileManager) Close(_ context.Context) error {
	if lfm.baseDirectoryRoot != nil {
		return lfm.baseDirectoryRoot.Close()
	}

	return nil
}

// ListItems returns a list of items in the storage backend.
func (lfm *FileManager) ListItems(_ context.Context) ([]string, error) {
	var items []string
	rootFS := lfm.baseDirectoryRoot.FS()

	err := fs.WalkDir(rootFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("failed on %q while walking directory %q: %w", path, lfm.baseDirectory, err)
		}

		// Collect matching non-directory items
		if !d.IsDir() {
			items = append(items, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %q: %w", lfm.baseDirectory, err)
	}

	return items, nil
}

// GetLocalFilePath gets a local filesystem path to the specified item.
func (lfm *FileManager) GetLocalFilePath(_ context.Context, item string) (string, error) {
	info, err := lfm.baseDirectoryRoot.Lstat(item)
	if err != nil {
		return "", fmt.Errorf("failed to verify that item %q exists: %w", item, err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("item %q is not a regular file", item)
	}

	return item, nil
}
