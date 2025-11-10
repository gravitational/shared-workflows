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

package apt

import (
	"io"
	"log/slog"
	"regexp"
	"testing"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ospackages/publishers/discard"
	"github.com/stretchr/testify/assert"
)

func TestWithRepos(t *testing.T) {
	filenameMatcherA := regexp.MustCompile("^regex A$")
	filenameMatcherB := regexp.MustCompile("^regex B$")

	providedRepos := map[string]map[string]map[string][]*regexp.Regexp{
		"repo a": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
		"repo b": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
	}

	// Same as provided but with regex deduped
	expectedRepos := map[string]map[string]map[string][]*regexp.Regexp{
		"repo a": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
		"repo b": {
			"distribution a": {
				"component a": {
					filenameMatcherA,
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
				},
			},
			"distribution b": {
				"component a": {
					filenameMatcherB,
				},
				"component b": {
					filenameMatcherA,
					filenameMatcherB,
				},
			},
		},
	}

	manager := NewManager(nil, WithRepos(providedRepos))
	assert.Equal(t, expectedRepos, manager.repos)
}

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
			runner := &Manager{}

			opt := WithLogger(tt.providedLogger)
			opt(runner)

			assert.EqualValues(t, tt.expectedLogger, runner.logger)
		})
	}
}

func TestWithPublisher(t *testing.T) {
	testtPublisher := discard.NewDiscardPublisher()

	tests := []struct {
		name              string
		providedPublisher ospackages.APTPublisher
		expectedPublisher ospackages.APTPublisher
	}{
		{
			name:              "with nil publisher",
			expectedPublisher: discard.DiscardPublisher,
		},
		{
			name:              "with new publisher",
			providedPublisher: testtPublisher,
			expectedPublisher: testtPublisher,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Manager{}

			opt := WithPublisher(tt.providedPublisher)
			opt(runner)

			assert.EqualValues(t, tt.expectedPublisher, runner.publisher)
		})
	}
}
