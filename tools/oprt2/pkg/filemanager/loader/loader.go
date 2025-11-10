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

// This must be in a separate package to prevent a circular dep between the filemanager interface and implementation
package loader

import (
	"context"
	"fmt"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager/local"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager/s3"
)

// FromConfig builds a new file manager from the provided config.
func FromConfig(ctx context.Context, config config.FileManager) (filemanager.FileManager, error) {
	switch {
	case config.Local != nil:
		return local.FromConfig(config.Local)
	case config.S3 != nil:
		return s3.FromConfig(ctx, config.S3)
	default:
		return nil, fmt.Errorf("no file manager specified")
	}
}
