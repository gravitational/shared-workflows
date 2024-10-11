/*
 *  Copyright 2023 Gravitational, Inc
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

package loaders

// Defines a type that can load environment values from raw byte sources
type Loader interface {
	// Returns true if the byte array can be converted into a key/value map
	CanGetEnvironmentValues([]byte) bool
	// Converts the byte array into a key/value map
	GetEnvironmentValues([]byte) (map[string]string, error)
	// Human-readable name of the loader
	Name() string
}

// Generic loader that can load values of any supported format
var DefaultLoader = NewSubLoader(NewYamlLoader())
