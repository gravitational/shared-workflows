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
	"fmt"
	"os"
	"path/filepath"

	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/locking"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// S3FileManager is a [filemanager.FileManager] that retrieves files from S3.
type S3FileManager struct {
	workingDirectory     string
	workingDirectoryRoot *os.Root
	s3Client             *s3.Client
	bucket               string
	bucketPrefix         string
	itemLocker           locking.MutexMap[string]
}

var _ filemanager.ClosableFileManager = &S3FileManager{}

type S3FileManagerOption func(sfm *S3FileManager)

// WithPathPrefix specifies that only objects with a specific prefix should be returned.
func WithPathPrefix(pathPrefix string) S3FileManagerOption {
	return func(sfm *S3FileManager) {
		sfm.bucketPrefix = pathPrefix
	}
}

// NewS3FileManager creates a new S3FileManager with the specified bucket name and S3 client.
func NewS3FileManager(s3Client *s3.Client, bucket string, opts ...S3FileManagerOption) (sfm *S3FileManager, err error) {
	sfm = &S3FileManager{
		s3Client: s3Client,
		bucket:   bucket,
	}
	defer func() {
		if err == nil {
			return
		}
		err = errors.Join(err, sfm.Close())
	}()

	for _, opt := range opts {
		opt(sfm)
	}

	sfm.workingDirectory, err = os.MkdirTemp("", "s3-local-storage")
	if err != nil {
		return nil, fmt.Errorf("failed to create local working directory for S3 file manager")
	}

	sfm.workingDirectoryRoot, err = os.OpenRoot(sfm.workingDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to open working directory %q: %w", sfm.workingDirectory, err)
	}

	return sfm, nil
}

func (sfm *S3FileManager) Name() string {
	return "s3@" + sfm.bucket
}

func (sfm *S3FileManager) Close() error {
	// Closing this first ensures that item retrieval will not be attempted while removing the directories
	sfm.itemLocker.Close()

	cleanupErrs := make([]error, 0, 2)
	if sfm.workingDirectoryRoot != nil {
		if err := sfm.workingDirectoryRoot.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to close working directory root: %w", err))
		}
	}

	if sfm.workingDirectory != "" {
		if err := os.RemoveAll(sfm.workingDirectory); err != nil {
			cleanupErrs = append(cleanupErrs,
				fmt.Errorf("failed to remove working directory at %q: %w", sfm.workingDirectory, err))
		}
	}

	return errors.Join(cleanupErrs...)
}

// ListItemsWithPrefix returns a list of items in the storage backend that match the given prefix.
func (sfm *S3FileManager) ListItems(ctx context.Context) ([]string, error) {
	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket: &sfm.bucket,
	}

	if sfm.bucketPrefix != "" {
		listObjectsInput.Prefix = &sfm.bucketPrefix
	}

	paginator := s3.NewListObjectsV2Paginator(sfm.s3Client, listObjectsInput)

	var items []string
	for paginator.HasMorePages() {
		resp, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list items with prefix %q in bucket %q: %w", sfm.bucketPrefix, sfm.bucket, err)
		}

		for _, item := range resp.Contents {
			if item.Key != nil {
				items = append(items, *item.Key)
			}
		}
	}

	return items, nil
}

// GetLocalFilePath gets a local filesystem path to the specified item.
func (sfm *S3FileManager) GetLocalFilePath(ctx context.Context, item string) (localFilePath string, err error) {
	if err := sfm.itemLocker.Lock(ctx, item); err != nil {
		return "", err
	}

	// Ensure that the lock is always released to prevent blocking other callers infinitely
	defer sfm.itemLocker.Unlock(context.TODO(), item)

	switch _, err := sfm.workingDirectoryRoot.Stat(item); {
	case err == nil:
		// Item has already been downloaded
		return item, nil
	case !os.IsNotExist(err):
		return "", fmt.Errorf("failed to check if item %q has already been downloaded: %w", item, err)
	}

	// Item is missing, download it
	fullLocalFilePath := filepath.Join(sfm.workingDirectory, item)
	localFile, err := sfm.workingDirectoryRoot.Create(item)
	if err != nil {
		return "", fmt.Errorf("failed to create local file for item %q at %q: %w", item, fullLocalFilePath, err)
	}
	defer func() {
		closeErr := localFile.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("failed to close local file at %q: %w", fullLocalFilePath, err)
		}
		err = errors.Join(err, closeErr)
	}()

	// Build the downloader
	downloader := s3manager.NewDownloader(sfm.s3Client)
	downloader.Logger = logging.ToAWSLogger(logging.FromCtx(ctx))

	// Download the item
	if _, err := downloader.Download(ctx, localFile, &s3.GetObjectInput{Bucket: &sfm.bucket, Key: &item}); err != nil {
		return "", fmt.Errorf("failed to download item %q from bucket %q to %q: %w", item, sfm.bucket, fullLocalFilePath, err)
	}

	return fullLocalFilePath, nil
}
