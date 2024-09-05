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
package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v63/github"
	"github.com/gravitational/trace"
)

var SearchTimeNow = time.Unix(0, 0)

// %Y-%m-%dT%H:%M:%S%z
const searchTimeLayout = "2006-01-02T15:04:05-0700"

// ChangelogPR contains all the data necessary for a changelog from the PR
type ChangelogPR struct {
	Body   string `json:"body,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty" graphql:"url"`
}

// ListChangelogPullRequestsOpts contains options for searching for changelog pull requests.
type ListChangelogPullRequestsOpts struct {
	Branch   string
	FromDate time.Time
	ToDate   time.Time
}

// ListChangelogPullRequests will search for pull requests that provide changelog information.
func (c *Client) ListChangelogPullRequests(ctx context.Context, org, repo string, opts *ListChangelogPullRequestsOpts) ([]ChangelogPR, error) {
	var prs []ChangelogPR
	query := fmt.Sprintf(`repo:%s/%s base:%s merged:%s -label:no-changelog`,
		org, repo, opts.Branch, dateRangeFormat(opts.FromDate, opts.ToDate))
	page, _, err := c.client.Search.Issues(
		ctx,
		query,
		&github.SearchOptions{
			Sort:      "",
			Order:     "",
			TextMatch: false,
			ListOptions: github.ListOptions{
				Page:    0,
				PerPage: 100,
			},
		},
	)

	if err != nil {
		return prs, trace.Wrap(err)
	}

	for _, pull := range page.Issues {
		prs = append(prs, ChangelogPR{
			Body:   pull.GetBody(),
			Number: pull.GetNumber(),
			Title:  pull.GetTitle(),
			URL:    pull.GetHTMLURL(),
		})
	}

	return prs, nil
}

// dateRangeFormat takes in a date range and will format it for GitHub search syntax.
// to can be empty and the format will be to search everything after from
func dateRangeFormat(from, to time.Time) string {
	if to == SearchTimeNow {
		return fmt.Sprintf(">%s", from.Format(searchTimeLayout))
	}
	return fmt.Sprintf("%s..%s", from.Format(searchTimeLayout), to.Format(searchTimeLayout))
}
