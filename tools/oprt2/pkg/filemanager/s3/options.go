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

package s3

import (
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

type S3FileManagerOption func(sfm *FileManager)

// WithPathPrefix specifies that only objects with a specific prefix should be returned.
func WithPathPrefix(pathPrefix string) S3FileManagerOption {
	return func(sfm *FileManager) {
		sfm.bucketPrefix = pathPrefix
	}
}

// WithLogger configures the file manager with the provided logger.
func WithLogger(logger *slog.Logger) S3FileManagerOption {
	return func(sfm *FileManager) {
		if logger == nil {
			logger = logging.DiscardLogger
		}
		sfm.logger = logger
	}
}
