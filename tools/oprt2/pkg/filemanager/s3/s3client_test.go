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

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
)

type mockS3Client struct {
	mock.Mock
}

var _ S3Client = (*mockS3Client)(nil)

func (s3c *mockS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := []any{ctx, input}
	for _, opt := range opts {
		args = append(args, opt)
	}

	ret := s3c.Called(args...)

	return ret.Get(0).(*s3.GetObjectOutput), ret.Error(1)
}

func (s3c *mockS3Client) ListObjectsV2(ctx context.Context, input *s3.ListObjectsV2Input, opts ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	args := []any{ctx, input}
	for _, opt := range opts {
		args = append(args, opt)
	}

	ret := s3c.Called(args...)

	return ret.Get(0).(*s3.ListObjectsV2Output), ret.Error(1)
}
