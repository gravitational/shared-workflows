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
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/gravitational/trace"
)

// PullRequest is a pull request in a GitHub repository.
type PullRequest struct {
	Body   string `json:"body"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

// prBatchSize is the number of PRs fetched per GraphQL request, sized to
// keep each query well within GitHub's cost limits.
const prBatchSize = 50

// PullRequests fetches the given PRs by number, returning them in the order
// given. PRs that no longer exist (e.g. deleted) are omitted from the result.
func (c *Client) PullRequests(ctx context.Context, org, repo string, numbers []int) ([]PullRequest, error) {
	var result []PullRequest
	for chunk := range slices.Chunk(numbers, prBatchSize) {
		batch, err := c.fetchPRBatch(ctx, org, repo, chunk)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		result = append(result, batch...)
	}
	return result, nil
}

func prAlias(i int) string { return fmt.Sprintf("pr_%d", i) }

func (c *Client) fetchPRBatch(ctx context.Context, org, repo string, numbers []int) ([]PullRequest, error) {
	var sb strings.Builder
	sb.WriteString(`query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) {`)
	for i, num := range numbers {
		fmt.Fprintf(&sb, ` %s: pullRequest(number: %d) { body number title url }`, prAlias(i), num)
	}
	sb.WriteString(` } }`)
	variables := map[string]any{"owner": org, "name": repo}

	var response struct {
		Repository map[string]*PullRequest `json:"repository"`
	}

	// The API reports a missing PR as a NOT_FOUND error alongside the rest
	// of the data, with null for the PR itself. Tolerate those; fail on
	// anything else.
	if err := c.graphql.DoWithContext(ctx, sb.String(), variables, &response); err != nil {
		var gqlErr *api.GraphQLError
		if !errors.As(err, &gqlErr) || !gqlErr.Match("NOT_FOUND", "repository.") {
			return nil, trace.Wrap(err)
		}
	}

	prs := make([]PullRequest, 0, len(numbers))
	for i := range numbers {
		if pr := response.Repository[prAlias(i)]; pr != nil {
			prs = append(prs, *pr)
		}
	}
	return prs, nil
}

// commitPageSize is the number of a PR's commits fetched per page. GitHub
// caps connection pages at 100.
const commitPageSize = 100

// PullRequestCommitMessages returns the full commit messages of the commits in
// each of the given PRs, keyed by PR number. PRs that no longer exist are
// omitted.
//
// This recovers what a squash merge discards. A squash-merged PR collapses its
// commits into one, so the constituent commits are absent from local history,
// and the squash commit's message is the only local record of them - a record
// that is lost entirely when the message is trimmed to just the PR title.
// Asking GitHub for the PR's commits gets them back.
//
// The messages are returned whole rather than reduced to subjects, because a
// commit's subject is not always enough to identify the changes in it: squashed
// commits routinely bundle several changes, listing all but the first in the
// message body.
func (c *Client) PullRequestCommitMessages(ctx context.Context, org, repo string, numbers []int) (map[int][]string, error) {
	messages := make(map[int][]string, len(numbers))
	for _, number := range numbers {
		m, err := c.fetchPRCommitMessages(ctx, org, repo, number)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if m != nil {
			messages[number] = m
		}
	}
	return messages, nil
}

// prCommitsQuery requests whole commit messages. Note that messageHeadline is
// not a usable substitute for the first line of message: GitHub truncates it
// with an ellipsis, so long subjects come back cut off and compare unequal to
// the same subject read from git.
const prCommitsQuery = `query($owner: String!, $name: String!, $number: Int!, $pageSize: Int!, $after: String) {
	repository(owner: $owner, name: $name) {
		pullRequest(number: $number) {
			commits(first: $pageSize, after: $after) {
				pageInfo { hasNextPage endCursor }
				nodes { commit { message } }
			}
		}
	}
}`

func (c *Client) fetchPRCommitMessages(ctx context.Context, org, repo string, number int) ([]string, error) {
	var messages []string
	var after *string

	// Bound the paging so a malformed response cannot spin forever.
	for range 50 {
		var response struct {
			Repository struct {
				PullRequest *struct {
					Commits struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							Commit struct {
								Message string `json:"message"`
							} `json:"commit"`
						} `json:"nodes"`
					} `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}

		variables := map[string]any{
			"owner":    org,
			"name":     repo,
			"number":   number,
			"pageSize": commitPageSize,
			"after":    after,
		}

		// A missing PR comes back as a NOT_FOUND error alongside a null
		// pullRequest. Tolerate it; fail on anything else.
		if err := c.graphql.DoWithContext(ctx, prCommitsQuery, variables, &response); err != nil {
			var gqlErr *api.GraphQLError
			if !errors.As(err, &gqlErr) || !gqlErr.Match("NOT_FOUND", "repository.") {
				return nil, trace.Wrap(err, "can't get commits for PR #%d", number)
			}
		}

		pr := response.Repository.PullRequest
		if pr == nil {
			return nil, nil
		}
		for _, node := range pr.Commits.Nodes {
			messages = append(messages, node.Commit.Message)
		}
		if !pr.Commits.PageInfo.HasNextPage {
			return messages, nil
		}
		cursor := pr.Commits.PageInfo.EndCursor
		after = &cursor
	}

	return nil, trace.LimitExceeded("too many commit pages for PR #%d", number)
}
