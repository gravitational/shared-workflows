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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestNewFileManager(t *testing.T) {
	s3Client := &mockS3Client{}

	fileManager, err := NewFileManager(s3Client, "test-bucket")
	t.Cleanup(func() {
		if fileManager != nil {
			assert.NoError(t, fileManager.Close(context.TODO()))
		}
	})

	require.NoError(t, err)
	require.NotNil(t, fileManager)
	assert.Equal(t, logging.DiscardLogger, fileManager.logger)
	assert.NotNil(t, fileManager.workingDirectoryRoot)
	assert.Equal(t, s3Client, fileManager.s3Client)
}

func TestName(t *testing.T) {
	s3Client := &mockS3Client{}
	bucketName := "some-bucket"

	fileManager, err := NewFileManager(s3Client, bucketName)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, fileManager.Close(context.TODO())) })

	name := fileManager.Name()
	assert.Contains(t, name, "s3")
	assert.Contains(t, name, bucketName)
}

func TestClose(t *testing.T) {
	s3Client := &mockS3Client{}
	bucketName := "some-bucket"

	fileManager, err := NewFileManager(s3Client, bucketName)
	require.NoError(t, err)

	err = fileManager.Close(t.Context())
	require.NoError(t, err)
	assert.NoDirExists(t, fileManager.workingDirectory)
}

func TestListItems(t *testing.T) {
	ctx := t.Context()
	itemA := "item A"
	itemB := "item/B"
	bucketName := "some-bucket"

	s3Client := &mockS3Client{}
	s3Client.On("ListObjectsV2", ctx, mock.Anything, mock.Anything).Return(
		&s3.ListObjectsV2Output{
			Contents: []s3types.Object{
				{
					Key: ptr.String(itemA),
				},
				{
					Key: ptr.String(itemB),
				},
			},
		},
		nil,
	).Once()

	fileManager, err := NewFileManager(s3Client, bucketName, WithPathPrefix("some/path/prefix/"))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, fileManager.Close(context.TODO())) })

	items, err := fileManager.ListItems(ctx)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{itemA, itemB}, items)
}

func TestGetLocalFilePath(t *testing.T) {
	ctx := t.Context()
	itemName := "item/name"
	bucketName := "some-bucket"
	fileContents := "file contents"

	s3Client := &mockS3Client{}
	// GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	s3Client.On("GetObject", ctx, mock.Anything, mock.Anything).Return(
		&s3.GetObjectOutput{
			Body:          io.NopCloser(strings.NewReader(fileContents)),
			ContentLength: ptr.Int64(int64(len(fileContents))),
		},
		nil,
	).Once()

	fileManager, err := NewFileManager(s3Client, bucketName, WithPathPrefix("some/path/prefix/"))
	require.NoError(t, err)

	var localFilePath string
	t.Cleanup(func() {
		assert.NoError(t, fileManager.Close(context.TODO()))
		assert.NoFileExists(t, localFilePath)
	})

	localFilePath, err = fileManager.GetLocalFilePath(ctx, itemName)
	require.NoError(t, err)
	require.FileExists(t, localFilePath)
	readFileContents, err := os.ReadFile(localFilePath)
	require.NoError(t, err)
	assert.Equal(t, fileContents, string(readFileContents))

	// This + the `Once()` on `GetObject()` will verify that the file isn't downloaded multiple times.
	secondCallFilePath, err := fileManager.GetLocalFilePath(ctx, itemName)
	require.NoError(t, err)
	require.FileExists(t, localFilePath)
	assert.Equal(t, localFilePath, secondCallFilePath)
}
