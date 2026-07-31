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
