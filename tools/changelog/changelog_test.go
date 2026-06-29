/*
Copyright 2024 Gravitational, Inc.

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
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/stretchr/testify/assert"
)

func TestChangelogScraper(t *testing.T) {
	// Test the changelog scraper functionality
	scraper := &changelogScraper{
		repo: "test-repo",
		ghclient: &fakeGitHubClient{
			testFileName: "testdata/listed-prs.json",
		},
	}

	data, err := scraper.scrapeForChangelogs(t.Context(), "master", time.Now(), time.Now())
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Len(t, data, 23)

	coreCLCount := 0
	entCLCount := 0
	for _, item := range data {
		if !item.IsEnterprise {
			coreCLCount++
		} else {
			entCLCount++
		}
	}
	assert.Equal(t, 20, coreCLCount)
	assert.Equal(t, 3, entCLCount)
}

func TestFindChangelogLinesCoreAndEnterpriseMarkers(t *testing.T) {
	body := `
- changelog: core fix
* changelog-enterprise: paid feature
`

	assert.Equal(t, []string{"core fix"}, findChangelogLines(body, clPattern))
	assert.Equal(t, []string{"paid feature"}, findChangelogLines(body, entCLPattern))
}

func TestExtractChangelogsFromPREnterpriseOnly(t *testing.T) {
	pr := github.ChangelogPR{
		Title:  "Fallback title",
		Number: 42,
		URL:    "https://example.test/pull/42",
		Body:   "changelog-enterprise: enterprise behavior",
	}

	scraper := &changelogScraper{}

	items := scraper.extractChangelogsFromPR(pr)
	assert.Len(t, items, 1)
	assert.Equal(t, "Enterprise behavior.", items[0].Summary)
	assert.True(t, items[0].IsEnterprise)
}
