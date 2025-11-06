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

// FileManager defines a storage backend for storing and pulling files.
type FileManager struct {
	// Not implemented
}

// GPGProvider defines a source of GPG keys.
type GPGProvider struct {
	// Not implemented
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
	// If this is used, then the global Attune configuration must also be provided.
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
	// Components are the APT repo components that should be used.
	// Key is the name of the APT component, value is a list of regular expressions
	// that match file path that should be associated with the component.
	Components map[string][]string
	// Distros are the APT distros that should be used.
	// Key is the name of the distro (e.g. `ubuntu`, `debian`), value is a list of
	// distro versions (e.g. `plucky`, `trixie`).
	Distros map[string][]string
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
