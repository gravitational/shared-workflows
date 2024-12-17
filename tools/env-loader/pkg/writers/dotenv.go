/*
 *  Copyright 2024 Gravitational, Inc
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

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/gravitational/trace"
)

// Pulled from https://hexdocs.pm/dotenvy/dotenv-file-format.html#variable-names
var dotEnvKeyValidationRegex = regexp.MustCompile("^[a-zA-Z_]+[a-zA-Z0-9_]*$")

// Outputs the values in .env file format
type DotenvWriter struct{}

// Create a new dotenv-format writer
func NewDotenvWriter() *DotenvWriter {
	return &DotenvWriter{}
}

func (*DotenvWriter) validateValue(key, _ string) error {
	if dotEnvKeyValidationRegex.MatchString(key) {
		return nil
	}

	return trace.Errorf("Environment value name %q cannot be written to a dotenv file", key)
}

func (ew *DotenvWriter) FormatEnvironmentValues(values map[string]values.Value) (string, error) {
	lines := make([]string, 0, len(values))
	for key, value := range values {
		err := ew.validateValue(key, value.UnderlyingValue)
		if err != nil {
			return "", trace.Wrap(err)
		}

		fileLine := fmt.Sprintf("%s=%s\n", key, value.UnderlyingValue)
		lines = append(lines, fileLine)
	}

	return strings.Join(lines, ""), nil
}

func (*DotenvWriter) Name() string {
	return "dotenv"
}
