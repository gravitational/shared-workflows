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
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

func FromConfig(ctx context.Context, config *config.S3FileManager) (*FileManager, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from default sources: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	sfm, err := NewFileManager(client, config.Bucket, WithPathPrefix(config.Prefix))
	if err != nil {
		return nil, fmt.Errorf("failed to build S3 file manager: %w", err)
	}

	return sfm, nil
}
