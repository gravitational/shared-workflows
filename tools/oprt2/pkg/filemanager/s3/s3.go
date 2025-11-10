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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/filemanager"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/locking"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// S3Client is a reduced version of s3.Client that only defines required methods.
// This makes testing significantly easier.
type S3Client interface {
	s3manager.DownloadAPIClient
	s3.ListObjectsV2APIClient
}

// FileManager is a [filemanager.FileManager] that retrieves files from S3.
type FileManager struct {
	workingDirectory     string
	workingDirectoryRoot *os.Root
	s3Client             S3Client
	bucket               string
	bucketPrefix         string
	itemLocker           *locking.MutexMap[string]
	logger               *slog.Logger
}

var _ filemanager.FileManager = &FileManager{}

// NewFileManager creates a new S3FileManager with the specified bucket name and S3 client.
func NewFileManager(s3Client S3Client, bucket string, opts ...S3FileManagerOption) (sfm *FileManager, err error) {
	sfm = &FileManager{
		s3Client:   s3Client,
		bucket:     bucket,
		itemLocker: locking.NewMutexMap[string](),
		logger:     logging.DiscardLogger,
	}
	defer func() {
		if err == nil {
			return
		}
		err = errors.Join(err, sfm.Close(context.TODO()))
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

func (sfm *FileManager) Name() string {
	return "s3@" + sfm.bucket
}

func (sfm *FileManager) Close(ctx context.Context) error {
	// Closing this first ensures that item retrieval will not be attempted while removing the directories
	sfm.itemLocker.Close(ctx)

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
func (sfm *FileManager) ListItems(ctx context.Context) ([]string, error) {
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
func (sfm *FileManager) GetLocalFilePath(ctx context.Context, item string) (localFilePath string, err error) {
	if err := sfm.itemLocker.Lock(ctx, item); err != nil {
		return "", err
	}

	// Ensure that the lock is always released to prevent blocking other callers infinitely
	defer func() { _ = sfm.itemLocker.Unlock(context.TODO(), item) }()

	fullLocalFilePath := filepath.Join(sfm.workingDirectory, item)

	switch _, err := sfm.workingDirectoryRoot.Stat(item); {
	case err == nil:
		// Item has already been downloaded
		return fullLocalFilePath, nil
	case !os.IsNotExist(err):
		return "", fmt.Errorf("failed to check if item %q has already been downloaded: %w", item, err)
	}

	// Item is missing, download it
	itemParentPath := filepath.Dir(item)
	if err := sfm.workingDirectoryRoot.MkdirAll(itemParentPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create local directory for item %q at %q: %w", item, itemParentPath, err)
	}

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
	downloader.Logger = logging.ToAWSLogger(sfm.logger)

	// Download the item
	if _, err := downloader.Download(ctx, localFile, &s3.GetObjectInput{Bucket: &sfm.bucket, Key: &item}); err != nil {
		return "", fmt.Errorf("failed to download item %q from bucket %q to %q: %w", item, sfm.bucket, fullLocalFilePath, err)
	}

	return fullLocalFilePath, nil
}
