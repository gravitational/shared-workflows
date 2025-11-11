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
	"io"
	"log/slog"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/stretchr/testify/assert"
)

func TestWithLogger(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name           string
		providedLogger *slog.Logger
		expectedLogger *slog.Logger
	}{
		{
			name:           "with nil logger",
			expectedLogger: logging.DiscardLogger,
		},
		{
			name:           "with new logger",
			providedLogger: testLogger,
			expectedLogger: testLogger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &Provider{}

			opt := WithLogger(tt.providedLogger)
			opt(provider)

			assert.EqualValues(t, tt.expectedLogger, provider.logger)
		})
	}
}

func TestWithKeyID(t *testing.T) {
	provider := &Provider{}
	keyID := "test key ID"

	opt := WithKeyID(keyID)
	opt(provider)

	assert.Equal(t, keyID, provider.keyID)
}
