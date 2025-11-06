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

package attune

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner/exec"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPublisher(t *testing.T) {
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name           string
		opts           []PublisherOpt
		expectedLogger *slog.Logger
	}{
		{
			name: "basic, no logger",
		},
		{
			name:           "with logger",
			expectedLogger: testLogger,
			opts: []PublisherOpt{
				WithLogger(testLogger),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedLogger == nil {
				tt.expectedLogger = logging.DiscardLogger
			}

			attuneRunner := exec.NewRunner()

			publisher := NewPublisher(attuneRunner)
			require.NotNil(t, publisher)
			assert.Equal(t, attuneRunner, publisher.attune)
		})
	}
}

func TestName(t *testing.T) {
	assert.NotEmpty(t, NewPublisher(nil).Name())
}

func TestPublishToAPTRepo(t *testing.T) {
	ctx := t.Context()
	repo := "test repo"
	distro := "test distro"
	component := "test component"
	packageFilePath := "test package file path"

	mockAttuneRunner := &mockRunner{}
	mockAttuneRunner.On("Run", ctx, "attune", "apt", "package", "add",
		"--repo", repo,
		"--distribution", distro,
		"--component", component,
		packageFilePath,
	).Return(nil).Once()

	publisher := NewPublisher(mockAttuneRunner)
	publisher.PublishToAPTRepo(ctx, repo, distro, component, packageFilePath)
}

func TestClose(t *testing.T) {
	assert.NoError(t, NewPublisher(nil).Close(t.Context()))
}
