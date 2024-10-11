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

package writers

// Writers take environment values and create a string (usually to be written
// to a file) of a given format.
type Writer interface {
	// Take in environment key/value pairs and format them.
	FormatEnvironmentValues(values map[string]string) (string, error)
	// Human-readable name of the writer, usually the output format.
	Name() string
}

var (
	dotenvWriter  = NewDotenvWriter()
	DefaultWriter = dotenvWriter

	// A map of all writers available.
	AllWriters = map[string]Writer{
		dotenvWriter.Name(): dotenvWriter,
	}
)
