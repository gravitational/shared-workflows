/*
Copyright 2026 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Overall this is mainly for testing that the templating is working as expected.
// This allows for faster iteration of the template itself and doesn't have much logic to test.
func TestRenderChangelog(t *testing.T) {
	testCases := []struct {
		name           string
		expectedFile   string
		excludePRLinks bool
	}{
		{
			name:           "include-links",
			expectedFile:   "expected-cl.md",
			excludePRLinks: false,
		},
		{
			name:           "exclude-links",
			expectedFile:   "expected-cl-no-links.md",
			excludePRLinks: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			scraper := &changelogScraper{
				repo: "test-repo",
				ghclient: &fakeGitHubClient{
					testFileName: "testdata/listed-prs.json",
				},
			}
			changelogs, err := scraper.scrapeForChangelogs(t.Context(), "master", time.Now(), time.Now())
			require.NoError(t, err)
			require.NotEmpty(t, changelogs)

			got, err := renderChangelog(renderOpts{
				changelogs:         changelogs,
				excludeCorePRLinks: tt.excludePRLinks,
			})
			require.NoError(t, err)
			require.NotEmpty(t, got)

			expected, err := os.ReadFile("testdata/" + tt.expectedFile)
			require.NoError(t, err)
			assert.Equal(t, string(expected), got)
		})
	}
}
