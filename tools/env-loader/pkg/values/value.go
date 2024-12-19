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

package values

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

const maxBcryptInputSize = 72

type Value struct {
	UnderlyingValue string
	ShouldMask      bool
}

// Returns a string representation of the value that is safe to log.
// Secret values will be truncated to 72 bytes and hashed with bcrypt, if
// possible. To help with debugging, users can check if a known value matches
// this hash with something like:
// go install github.com/shoenig/bcrypt-tool@latest
// bcrypt-tool match "expected value" '$2a$10$KnownValueHash...'
func (v *Value) String() string {
	if !v.ShouldMask {
		return v.UnderlyingValue
	}

	// Output value is salted and hashed to help with debugging without
	// exposing sensitive information, with a fallback to <redacted>
	// Truncate the value to 72 bytes
	truncatedValueBytes := []byte(v.UnderlyingValue)
	if len(truncatedValueBytes) > maxBcryptInputSize {
		truncatedValueBytes = truncatedValueBytes[0:maxBcryptInputSize]
	}

	hashedValue, err := bcrypt.GenerateFromPassword(truncatedValueBytes, bcrypt.DefaultCost)
	if err != nil {
		return "<redacted>"
	}

	return fmt.Sprintf("<redacted %s>", string(hashedValue))
}
