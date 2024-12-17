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

	"github.com/gravitational/shared-workflows/tools/env-loader/pkg/values"
	"github.com/gravitational/trace"
)

const ghaMaskPrefix = "::add-mask::"

// Outputs secret values prefixed with `::add-mask::`, one per line.
// Per GHA docs, this will prevent the values from being logged.
// For details, see
// https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions#masking-a-value-in-a-log
type GHAMaskWriter struct{}

// Create a new GHA mask writer
func NewGHAMaskWriter() *GHAMaskWriter {
	return &GHAMaskWriter{}
}

func (ew *GHAMaskWriter) FormatEnvironmentValues(values map[string]values.Value) (string, error) {
	renderedValues := make([]string, 0, len(values))
	for key, value := range values {
		if key == "" {
			// Log as much as possible without compromising security to help
			// with debugging. This could be further improved by hashing the
			// value.
			logValue := "<redacted>"
			if !value.ShouldMask {
				logValue = value.UnderlyingValue
			}

			return "", trace.Errorf("found empty key for log value %q", logValue)
		}

		if !value.ShouldMask {
			continue
		}

		renderedValues = append(renderedValues, maskLine(value.UnderlyingValue)...)
	}

	return strings.Join(renderedValues, ""), nil
}

// Mask a value. The value may contain new line characters.
// This is critically important for security, because GHA uses new line
// characters to mark the end of `::add-mask::` statements.
func maskLine(value string) []string {
	secretValueLines := strings.Split(value, "\n")
	formattedLines := make([]string, 0, len(secretValueLines))
	for _, valueLine := range secretValueLines {
		if valueLine == "" {
			continue
		}

		formattedLines = append(formattedLines, fmt.Sprintf("%s%s\n", ghaMaskPrefix, valueLine))
	}

	return formattedLines
}

func (*GHAMaskWriter) Name() string {
	return "gha-mask"
}
