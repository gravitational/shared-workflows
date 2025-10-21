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

// PackageManager defines package manager configuration.
type PackageManager struct {
	// Not implemented
}

// Attune defines Attune publishing configuration.
type Attune struct {
	// Authentication defines Attune authentication configuration.
	Authentication Authenticator
	// ParallelUploadLimit is the maximum number of packages to try to upload to the Attune
	// control plane at once. If unset, there will be no upload limit.
	ParallelUploadLimit uint
}

// Logger defines logging options for the tool.
type Logger struct {
	// Not implemented
}

// OPRT2 defines the tool's configuration.
type OPRT2 struct {
	Logger          *Logger
	Attune          Attune
	PackageManagers []PackageManager
}
