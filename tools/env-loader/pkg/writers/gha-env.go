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
	"strings"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/gravitational/trace"
)

const delimiterPrefix = "EOF"

// Outputs values in a format that can be parsed by GHA's `GITHUB_ENV` file.
// This is _almost_ the same as dotenv files, but also handles multiline
// environment values. For details, see
// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#setting-an-environment-variable
type GHAEnvWriter struct{}

// Create a new GHA env writer
func NewGHAEnvWriter() *GHAEnvWriter {
	return &GHAEnvWriter{}
}

// Generates a delimiter that is guaranteed to not contain the provided string.
// This is required for writing multiline values to `GITHUB_ENV`, per
// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#multiline-strings
func generateMultilineDelimiter(value string) string {
	valueLines := strings.Split(value, "\n")

	// Start with no suffix to make this a little more readable
	delimiter := delimiterPrefix
	for {
		// Check if there are any lines that match the delimiter exactly
		foundMatch := false
		for _, line := range valueLines {
			if line == delimiter {
				foundMatch = true
				break
			}
		}

		if foundMatch {
			// Add a reasonably unique value to the delimiter
			delimiter = fmt.Sprintf("%s_%s", delimiterPrefix, uuid.NewString())
			continue
		}

		// If no line matches the delimiter exactly, then the delimiter can be
		// used.
		return delimiter
	}
}

func (ew *GHAEnvWriter) FormatEnvironmentValues(values map[string]values.Value) (string, error) {
	renderedValues := make([]string, 0, len(values))
	for key, value := range values {
		if key == "" {
			return "", trace.Errorf("found empty key for log value %q", value.String())
		}

		// Don't format strings without new lines as multiline. This would be valid, but a little
		// less readable.
		var renderedValue string
		if strings.Contains(value.UnderlyingValue, "\n") {
			// Match GHA docs:
			// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#multiline-strings
			// Formats values like:
			// {name}<<{delimiter}
			// {value}
			// {delimiter}
			//
			delimiter := generateMultilineDelimiter(value.UnderlyingValue)
			renderedValue = fmt.Sprintf("%s<<%s\n%s\n%s\n", key, delimiter, value.UnderlyingValue, delimiter)
		} else {
			renderedValue = fmt.Sprintf("%s=%s\n", key, value.UnderlyingValue)
		}

		renderedValues = append(renderedValues, renderedValue)
	}

	return strings.Join(renderedValues, ""), nil
}

func (*GHAEnvWriter) Name() string {
	return "gha-env"
}
