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

package filemanager

import (
	"context"
	"errors"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// Handles storage and retrieval of items in a storage backend.
// A storage backend is a system (e.g. filesystem, S3, etc.) responsible for storing retrieving data.
// "Items" are pieces of data that can be stored and retrieved.
// Paths with ".." are not allowed, and may result in unexpected behavior.
type FileManager interface {
	// ListItems returns a list of items available in the storage backend.
	// Note: returned items may have a "/" in them, e.g. "foo/bar/baz".
	ListItems(ctx context.Context) ([]string, error)

	// GetLocalFilePath gets a filesystem path to the specified item. Other programs should be
	// able to read the item from this path. If the item is stored in a remote storage backend (e.g. S3),
	// this may download it.
	GetLocalFilePath(ctx context.Context, item string) (string, error)

	// Name is the name of the file manager
	Name() string

	// Close closes the file manager. This should include a cleanup of any local temporary resources.
	Close() error
}

// FromConfig builds a new file manager from the provided config.
func FromConfig(ctx context.Context, config config.FileManager) (FileManager, error) {
	return nil, errors.New("not implemented")
}
