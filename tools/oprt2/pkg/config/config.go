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

package config

// Authenticator defines Attune authentication configuration.
type Authenticator struct {
	// Not implemented
}

// S3FileManager is a file manager that uses files stored in a remote S3 bucket.
type S3FileManager struct {
	// Bucket is the name of the S3 bucket containing files.
	Bucket string
	// Prefix is the path prefix of the bucket that all files should exist under.
	Prefix string
}

// LocalFileManager is a file manager that uses files already on the local filesystem.
type LocalFileManager struct {
	// Directory is the path to the local directory containing files.
	Directory string
}

// FileManager defines a storage backend for storing and pulling files.
// Only one field may be specified.
type FileManager struct {
	// Local handles files on the local filesystem.
	Local *LocalFileManager
	// S3 handles files in a remote S3 bucket.
	S3 *S3FileManager
}

// GPGArchiveProvider provides a GPG key  via a compressed tar archive that is a standard
// RFC 4648 base64-encoded string.
// Example command to generate this:
// tar --exclude='.#*' --exclude="*~" --exclude="S.*" -czf - ~/.gnupg/ | base64
type GPGArchiveProvider struct {
	// Archive is the base64-encoded tarball to use.
	Archive string
	// KeyID is the key within the archive to use. Optional if there is only one key.
	KeyID *string
}

// GPGKeyIDProvider provides a GPG key from the local filesystem specified by the
// provided GPG key ID.
type GPGKeyIDProvider struct {
	// KeyID is the key within the GPG home directory to use. Optional if there is only one key.
	KeyID *string
	// GPGHomeDirectory is the path to the GPG home directory containing GPG keys. Optional.
	GPGHomeDirectory *string
}

// GPGProvider defines a source of GPG keys.
// Only one field may be specified.
type GPGProvider struct {
	// Archive provides a GPG key from a base64-encoded tarball.
	Archive *GPGArchiveProvider
	// KeyID provides a GPG key from the local filesystem.
	KeyID *GPGKeyIDProvider
}

// AttuneAPTPackagePublisher publishes packages for an APT repo via Attune.
// If this is used, then the global Attune configuration must also be provided.
type AttuneAPTPackagePublisher struct {
	// Authentication defines Attune authentication configuration.
	Authentication Authenticator
	// GPG is how GPG keys will be retrieved for repo signing. Optional.
	GPG *GPGProvider
}

// DiscardAPTPackagePublisher doesn't actually publish any APT packages.
// Useful for dry-runs.
type DiscardAPTPackagePublisher struct{}

// APTPackagePublisher defines the package publisher configuration.
// Only one field may be specified.
type APTPackagePublisher struct {
	// AttuneAPTPackagePublisher publishes packages for an APT repo via Attune.
	Attune *AttuneAPTPackagePublisher
	// DiscardAPTPackagePublisher doesn't actually publish any APT packages.
	// Useful for dry-runs.
	Discard *DiscardAPTPackagePublisher
}

// APTPackageManager defines configuration for APTPackageManager repo management.
// For APTPackageManager repo documentation, see https://wiki.debian.org/DebianRepository/Format.
type APTPackageManager struct {
	// FileSource is where Debian packages (*.deb files) will be pulled from.
	FileSource FileManager
	// Repos is the full config for what APT repos that should be used.
	// First-level key is repo name (e.g. `ubuntu`, `debian` for Gravitational).
	// Second-level key is distribution (e.g. `plucky`, `trixie` for Gravitational).
	// Third-level key is component (e.g. `stable/rolling`, `stable/v18` for Gravitational).
	// Values are a list of regular expressions that match file paths that should be associated with
	// with repo/distribution/component combination.
	// See https://wiki.debian.org/DebianRepository/Format#Overview for details.
	Repos map[string]map[string]map[string][]string
	// PublishingTool is the tool that should be used to publish packages.
	PublishingTool APTPackagePublisher
}

// PackageManager defines package manager configuration.
// Only one field may be specified.
type PackageManager struct {
	// APT defines an APT repo that should be managed.
	APT *APTPackageManager
}

// Logger defines logging options for the tool.
type Logger struct {
	// Not implemented
}

// OPRT2 defines the tool's configuration.
type OPRT2 struct {
	// Logger is the logging options for the tool. Optional.
	Logger *Logger
	// PackageManagers are the package manager repos that should be  used.
	PackageManagers []PackageManager
	// ParallelLimit is the maximum number of parallel operations (e.g. uploads)
	// that can run at once. If unset, or set to 0, there will be no limit. Optional.
	ParallelLimit uint
}
