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
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// Provider is a [commandrunner.Hook] that provides access to a GPG key
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
type Provider struct {
	archive              string
	keyID                string
	credentialsDirectory string
	logger               *slog.Logger
}

var _ commandrunner.Hook = (*Provider)(nil)

func NewProvider(ctx context.Context, archive string, opts ...ProviderOption) (*Provider, error) {
	p := &Provider{
		archive: archive,
		logger:  logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(p)
	}

	if err := p.setup(ctx); err != nil {
		return nil, fmt.Errorf("failed to setup provider: %w", err)
	}

	return p, nil
}

func (p *Provider) Name() string {
	return "GPG archive provider"
}

func (p *Provider) setup(ctx context.Context) (err error) {
	// Attempt to decode, decompress, and extract the contents of the archive
	b64Decoder := base64.NewDecoder(base64.RawStdEncoding, bytes.NewReader([]byte(p.archive)))

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
	p.credentialsDirectory = credentialsDirectory

	defer func() {
		if err == nil {
			return
		}

		cleanupErr := p.Close(ctx)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("cleanup failed: %w", err)
		}
		err = errors.Join(err, cleanupErr)
	}()

	// Extract all contents
	if err := p.extractArchive(ctx, tarExtractor); err != nil {
		return fmt.Errorf("failed to extract all GPG archive contents to disk: %w", err)
	}

	return nil
}

// extractArchive extracts all tar archive contents to disk.
func (p *Provider) extractArchive(ctx context.Context, reader *tar.Reader) error {
	// Open a root to catch/block non-local paths
	// This behavior may not be needed in a future version of Go, see [tar.Reader.Next] for details
	credDirRoot, err := os.OpenRoot(p.credentialsDirectory)
	if err != nil {
		return fmt.Errorf("failed to open credentials directory %q for root: %w", p.credentialsDirectory, err)
	}
	defer func() {
		cleanupErr := credDirRoot.Close()
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("failed to close credentials root at %q: %w", p.credentialsDirectory, err)
		}
		err = errors.Join(err, cleanupErr)
	}()

	// Write all the the archive contents to disk
	for tarHeader, err := reader.Next(); err != nil; {
		if tarHeader == nil {
			continue
		}

		if err := p.createTarFilesytemObject(ctx, credDirRoot, tarHeader, reader); err != nil {
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
func (p *Provider) createTarFilesytemObject(ctx context.Context, root *os.Root, header *tar.Header, reader *tar.Reader) error {
	p.logger.DebugContext(ctx, "attempting extraction of %q from GPG archive to %q", header.Name, root.Name())

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
		p.logger.WarnContext(ctx,
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

func (p *Provider) Command(_ context.Context, cmd *exec.Cmd) error {
	providerFlags := []string{"--gpg-home-dir", p.credentialsDirectory, "--key-id", p.keyID}

	if len(cmd.Args) == 0 {
		cmd.Args = providerFlags
		return nil
	}

	// Split the file path from the flags
	filePath := cmd.Args[len(cmd.Args)-1]

	var flags []string
	if len(cmd.Args) > 1 {
		flags = cmd.Args[0 : len(cmd.Args)-2]
	}

	// Args = flags + provider flags + file name
	args := make([]string, len(flags)+len(providerFlags)+1)
	args = append(args, flags...)
	args = append(args, providerFlags...)
	args = append(args, filePath)

	cmd.Args = args
	return nil
}

func (p *Provider) Close(_ context.Context) error {
	if p.credentialsDirectory == "" {
		return nil
	}

	if err := os.RemoveAll(p.credentialsDirectory); err != nil {
		return fmt.Errorf("failed to cleanup credentials directory (private key may have been leaked!): %w", err)
	}

	return nil
}
