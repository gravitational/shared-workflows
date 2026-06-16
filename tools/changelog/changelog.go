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
	"context"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/trace"
)

// changelogInfo is used for the changelog template format.
type changelogInfo struct {
	// Summary is the changelog summary extracted from a PR
	Summary string
	Number  int
	URL     string
	// IsEnterprise indicates if the changelog is for an enterprise feature
	IsEnterprise bool
}

var (
	// clPattern will match a changelog format with the summary as a subgroup.
	// e.g. will match a line "changelog: this is a changelog" with subgroup "this is a changelog".
	clPattern = regexp.MustCompile(`[Cc]hangelog: +(.*)`)
	// entCLPattern will match an enterprise changelog format with the summary as a subgroup.
	// e.g. will match a line "changelog-enterprise: this is an enterprise changelog" with subgroup "this is an enterprise changelog".
	entCLPattern = regexp.MustCompile(`[Cc]hangelog-[Ee]nterprise: +(.*)`)
)

// changelogScraper is used to scrape changelog information from GitHub pull requests.
type changelogScraper struct {
	repo     string
	ghclient ghClient
	// markAllEnterprise indicates if all changelogs should be marked as enterprise
	markAllEnterprise bool
}

// ghClient is a small interface for the GitHub client to allow for easier testing.
type ghClient interface {
	ListChangelogPullRequests(ctx context.Context, owner, repo string, opts *github.ListChangelogPullRequestsOpts) ([]github.ChangelogPR, error)
}

// scrapeForChangelogs will search for pull requests between two points in time and attempt to extract changelog information from them.
// This only considers PRs that have been labeled with the "changelog" label. Other PRs will be ignored.
// In the case of no changelog lines found, this will use the PR title as the changelog with a NOCL prefix.
//
// Examples of changelog lines this will match:
//   - "changelog: this is a changelog"
//   - "changelog-enterprise: this is an enterprise changelog"
//
// Multiple changelog lines can be specified in a single PR.
func (c *changelogScraper) scrapeForChangelogs(ctx context.Context, branch string, fromTime, toTime time.Time) ([]changelogInfo, error) {
	// Search github for changelog pull requests
	prs, err := c.ghclient.ListChangelogPullRequests(
		ctx,
		"gravitational",
		c.repo,
		&github.ListChangelogPullRequestsOpts{
			Branch:   branch,
			FromDate: fromTime,
			ToDate:   toTime,
		},
	)
	if err != nil {
		return []changelogInfo{}, trace.Wrap(err)
	}

	changelogs := []changelogInfo{}
	for _, pr := range prs {
		changelogs = append(changelogs, c.extractChangelogsFromPR(pr)...)
	}

	return changelogs, nil
}

// extractChangelogsFromPR will attempt to extract changelog information from a given PR.
// This will look for changelog entries in the PR body and return a list of changelog information.
func (c *changelogScraper) extractChangelogsFromPR(pr github.ChangelogPR) []changelogInfo {
	var result []changelogInfo

	info := changelogInfo{
		Summary:      "NOCL: " + pr.Title, // default summary
		Number:       pr.Number,
		URL:          pr.URL,
		IsEnterprise: c.markAllEnterprise,
	}

	changelogLines := findChangelogLines(pr.Body, clPattern)
	for _, summary := range changelogLines {
		info.Summary = prettierSummary(summary)
		result = append(result, info)
	}

	entChangelogLines := findChangelogLines(pr.Body, entCLPattern)
	if len(entChangelogLines) > 0 {
		for _, summary := range entChangelogLines {
			info.Summary = prettierSummary(summary)
			info.IsEnterprise = true
			result = append(result, info)
		}
	}

	if len(result) == 0 {
		return []changelogInfo{info}
	}

	return result
}

// findChangelogLines will parse a body of a PR with a given pattern to find its changelogs.
func findChangelogLines(commentBody string, pattern *regexp.Regexp) []string {
	var result []string
	matches := pattern.FindAllStringSubmatch(commentBody, -1)
	for _, m := range matches {
		// If a match is found then we should get a non empty slice
		// 0 index will be the whole match including "changelog: *"
		// 1 index will be the subgroup match which does not include "changelog: "
		if len(m) > 1 {
			result = append(result, m[1])
		}
	}
	return result
}

func prettierSummary(cl string) string {
	// Clean whitespace and add a period at end
	cl = strings.TrimSpace(cl)
	if !strings.HasSuffix(cl, ".") {
		cl += "."
	}

	// Uppercase first letter
	r := []rune(cl)
	r[0] = unicode.ToUpper(r[0])
	cl = string(r)

	return cl
}
