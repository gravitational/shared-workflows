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
	keyID                string
	credentialsDirectory string
	logger               *slog.Logger
}

var _ commandrunner.Hook = (*Provider)(nil)

// NewProvider creates a new Provider.
func NewProvider(ctx context.Context, archive string, opts ...ProviderOption) (*Provider, error) {
	p := &Provider{
		logger: logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(p)
	}

	if err := p.setup(ctx, archive); err != nil {
		return nil, fmt.Errorf("failed to setup provider: %w", err)
	}

	return p, nil
}

// Name implements the [commandrunner.Hook] interface.
func (p *Provider) Name() string {
	return "GPG archive provider"
}

func (p *Provider) setup(ctx context.Context, archive string) (err error) {
	// Attempt to decode, decompress, and extract the contents of the archive
	b64Decoder := base64.NewDecoder(base64.RawStdEncoding, bytes.NewReader([]byte(archive)))

	gzipDecompressor, err := gzip.NewReader(b64Decoder)
	if err != nil {
		return fmt.Errorf("failed to create gzip decompressor for archive: %w", err)
	}

	cleanupFunc1 := func(retErr error) error {
		// The underlying close function can return a decompression-related error and needs to be checked
		cleanupErr := gzipDecompressor.Close()
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("failed to close gzip decompressor, decompression may have failed: %w", cleanupErr)
		}
		return errors.Join(retErr, cleanupErr)
	}

	tarExtractor := tar.NewReader(gzipDecompressor)

	// Create a directory to extract the archive to
	credentialsDirectory, err := os.MkdirTemp("", "gpghome-*")
	if err != nil {
		return cleanupFunc1(fmt.Errorf("failed to create temporary GPG home directory: %w", err))
	}
	p.credentialsDirectory = credentialsDirectory

	cleanupFunc2 := func(retErr error) error {
		cleanupErr := p.Close(ctx)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("cleanup failed: %w", err)
		}
		return errors.Join(cleanupFunc1(retErr), cleanupErr)
	}

	// Extract all contents
	if err := p.extractArchive(ctx, tarExtractor); err != nil {
		return cleanupFunc2(fmt.Errorf("failed to extract all GPG archive contents to disk: %w", err))
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
	for {
		tarHeader, err := reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The reader should return this error specifically when all archive items have been enumerated
				break
			}
			return fmt.Errorf("archive extraction failed: %w", err)
		}

		if tarHeader == nil {
			continue
		}

		if err := p.createTarFilesytemObject(ctx, credDirRoot, tarHeader, reader); err != nil {
			return fmt.Errorf("failed to create filesystem object from GPG archive: %w", err)
		}
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
		if err := root.MkdirAll(header.Name, 0700); err != nil {
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
	if err := root.MkdirAll(filepath.Dir(relFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", realPath, err)
	}

	// Create the actual file
	_, err := root.Lstat(relFilePath)
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

// Command implements the [commandrunner.Hook] interface.
func (p *Provider) Command(_ context.Context, cmd *exec.Cmd) error {
	providerFlags := []string{"--gpg-home-dir", p.credentialsDirectory}

	if p.keyID != "" {
		providerFlags = append(providerFlags, "--key-id", p.keyID)
	}

	if len(cmd.Args) == 0 {
		cmd.Args = providerFlags
		return nil
	}

	// Split the file path from the flags
	filePath := cmd.Args[len(cmd.Args)-1]

	var flags []string
	if len(cmd.Args) > 1 {
		flags = cmd.Args[0 : len(cmd.Args)-1]
	}

	// Args = flags + provider flags + file name
	args := make([]string, 0, len(flags)+len(providerFlags)+1)
	args = append(args, flags...)
	args = append(args, providerFlags...)
	args = append(args, filePath)

	cmd.Args = args
	return nil
}

// Close implements the [commandrunner.Hook] interface.
func (p *Provider) Close(_ context.Context) error {
	if p.credentialsDirectory == "" {
		return nil
	}

	if err := os.RemoveAll(p.credentialsDirectory); err != nil {
		return fmt.Errorf("failed to cleanup credentials directory (private key may have been leaked!): %w", err)
	}

	return nil
}
