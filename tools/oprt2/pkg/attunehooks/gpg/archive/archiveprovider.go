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

package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// ArchiveProvider is a [commandrunner.Hook] that provides access to a GPG key
// that is provided via a compressed tar archive that is a standard RFC 4648
// base64-encoded string.
// Example command to generate this:
// tar --exclude='.#*' --exclude="*~" --exclude="S.*" -czf - ~/.gnupg/ | base64
//
// If the key ID is not explicitly set, the provider attemps to pick a key from
// the archive, if possible.
//
// File ownership, access time, modified time, permission bits, and other
// metadata are intentionally ignored. Created filesystem objects will have mode
// 0600, and will be owned by the current user.
type ArchiveProvider struct {
	archive              string
	keyID                string
	credentialsDirectory string
}

var _ commandrunner.SetupHook = &ArchiveProvider{}
var _ commandrunner.PreCommandHook = &ArchiveProvider{}
var _ commandrunner.CleanupHook = &ArchiveProvider{}

type ArchiveProviderOption func(ap *ArchiveProvider)

// WithKeyID sets the key ID to provide Attune. If not set and only one key exists in the
// archive, Attune will attempt to use it. Required when multiple keys exist in the archive.
func WithKeyID(keyID string) ArchiveProviderOption {
	return func(ap *ArchiveProvider) {
		ap.keyID = keyID
	}
}

func NewArchiveProvider(archive string, opts ...ArchiveProviderOption) *ArchiveProvider {
	ap := &ArchiveProvider{
		archive: archive,
	}

	for _, opt := range opts {
		opt(ap)
	}

	return ap
}

func (ap *ArchiveProvider) Name() string {
	return "GPG archive provider"
}

func (ap *ArchiveProvider) Setup(ctx context.Context) (err error) {
	// Attempt to decode, decompress, and extract the contents of the archive
	b64Decoder := base64.NewDecoder(base64.RawStdEncoding, bytes.NewReader([]byte(ap.archive)))

	gzipDecompressor, err := gzip.NewReader(b64Decoder)
	if err != nil {
		return fmt.Errorf("failed to create gzip decompressor for archive: %w", err)
	}
	defer func() {
		// The underlying close function can return a decompression-related error and needs to be checked
		cleanupErr := gzipDecompressor.Close()
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("failed to close gzip decompressor, decompression may have failed: %w", err)
		}
		err = errors.Join(err, cleanupErr)
	}()

	tarExtractor := tar.NewReader(gzipDecompressor)

	// Create a directory to extract the archive to
	credentialsDirectory, err := os.MkdirTemp("", "gpghome-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary GPG home directory: %w", err)
	}
	ap.credentialsDirectory = credentialsDirectory

	defer func() {
		if err == nil {
			return
		}

		cleanupErr := ap.Cleanup(ctx)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("cleanup failed: %w", err)
		}
		err = errors.Join(err, cleanupErr)
	}()

	// Extract all contents
	if err := ap.extractArchive(ctx, tarExtractor); err != nil {
		return fmt.Errorf("failed to extract all GPG archive contents to disk: %w", err)
	}

	return nil
}

// extractArchive extracts all tar archive contents to disk.
func (ap *ArchiveProvider) extractArchive(ctx context.Context, reader *tar.Reader) error {
	// Open a root to catch/block non-local paths
	// This behavior may not be needed in a future version of Go, see [tar.Reader.Next] for details
	credDirRoot, err := os.OpenRoot(ap.credentialsDirectory)
	if err != nil {
		return fmt.Errorf("failed to open credentials directory %q for root: %w", ap.credentialsDirectory, err)
	}
	defer func() {
		cleanupErr := credDirRoot.Close()
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("failed to close credentials root at %q: %w", ap.credentialsDirectory, err)
		}
		err = errors.Join(err, cleanupErr)
	}()

	// Write all the the archive contents to disk
	for tarHeader, err := reader.Next(); err != nil; {
		if tarHeader == nil {
			continue
		}

		if err := ap.createTarFilesytemObject(ctx, credDirRoot, tarHeader, reader); err != nil {
			return fmt.Errorf("failed to create filesystem object from GPG archive: %w", err)
		}
	}

	// The reader should return this error specifically when all archive items have been enumerated
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("archive extraction failed: %w", err)
	}

	return nil
}

// createTarFilesytemObject handles the extraction of a filesystem object from the tar arhive.
func (ap *ArchiveProvider) createTarFilesytemObject(ctx context.Context, root *os.Root, header *tar.Header, reader *tar.Reader) error {
	logger := logging.FromCtx(ctx)
	logger.DebugContext(ctx, "attempting extraction of %q from GPG archive to %q", header.Name, root.Name())

	if filepath.IsAbs(header.Name) {
		return fmt.Errorf("absolute file paths within the GPG archive are not supported, found entry %q", header.Name)
	}

	fileMode := header.FileInfo().Mode()
	switch {
	case fileMode.IsRegular():
		tarFileContents := make([]byte, header.Size)
		// Read _exactly_ tarHeader.Size bytes and error if more or less are available
		if _, err := io.ReadFull(reader, tarFileContents); err != nil {
			return fmt.Errorf("failed to read tar contents (corrupt file?) for entry %q: %w", header.Name, err)
		}

		if err := createTarFile(root, header.Name, tarFileContents); err != nil {
			return fmt.Errorf("failed to create file %q from archive: %w", header.Name, err)
		}
	case fileMode.IsDir():
		if err := createTarDirectory(root, header.Name); err != nil {
			return fmt.Errorf("failed to create directory %q from archive: %w", header.Name, err)
		}
	default:
		logger.WarnContext(ctx,
			"credentials archive contains unsupported filesystem object of type %q at %q",
			fileMode.Type().String(), header.Name)
	}

	return nil
}

// createTarFile creates the file at the provided relative path with the provided contents.
// Missing parent directories are created.
// If a filesystem object already exists at the provided path, an error is returned and the
// object is not modified.
func createTarFile(root *os.Root, relFilePath string, contents []byte) error {
	realPath := filepath.Join(root.Name(), relFilePath)
	if err := createTarDirectory(root, relFilePath); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", realPath, err)
	}

	// Create the actual file
	_, err := root.Lstat(realPath)
	if err == nil {
		return fmt.Errorf("filesystem object already exists at %q (duplicate archive entries?)", realPath)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if file already exists at %q: %w", realPath, err)
	}

	file, err := root.OpenFile(relFilePath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %q for writing: %w", realPath, err)
	}

	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("failed to write file contents to %q: %w", realPath, err)
	}

	return nil
}

// createTarDirectory creates the directory at the provided relative path, as well as all parents.
// The directories will be owned by the current use and will have permission bits 0600 set.
func createTarDirectory(root *os.Root, relDirPath string) error {
	// Split up the parent directories so that each one can be created individually
	// The first item in this slice is always the topmost directory
	parentDirectories := make([]string, 0)
	directory := relDirPath
	for directory != "." {
		parentDirectories = append([]string{directory}, parentDirectories...)
		directory = filepath.Base(directory)
	}

	// Loop over the parent directories in reverse order. The slice contains the
	// top-levle directory _last_, not first.
	for _, parentDirectory := range parentDirectories {
		realPath := filepath.Join(root.Name(), parentDirectory)

		fsoInfo, err := root.Lstat(parentDirectory)
		if err == nil {
			// Just verify that filesystem object is a directory. This FSO should
			// have the correct permissions assuming that it was created by this
			// function
			if !fsoInfo.IsDir() {
				return fmt.Errorf("filesystem object already exists at %q but is not a directory (conflicting archive entries?)", realPath)
			}
			continue
		}

		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to lstat %q while extracting archive: %w", realPath, err)
		}

		if err := os.Mkdir(parentDirectory, 0600); err != nil {
			return fmt.Errorf("failed to create directory at %q: %w", realPath, err)
		}
	}

	return nil
}

func (ap *ArchiveProvider) PreCommand(_ context.Context, _ *string, args *[]string) error {
	*args = append([]string{"--gpg-home-dir", ap.credentialsDirectory, "--key-id", ap.keyID}, *args...)
	return nil
}

func (ap *ArchiveProvider) Cleanup(_ context.Context) error {
	if ap.credentialsDirectory == "" {
		return nil
	}

	if err := os.RemoveAll(ap.credentialsDirectory); err != nil {
		return fmt.Errorf("failed to cleanup credentials directory (private key may have been leaked!): %w", err)
	}

	return nil
}
