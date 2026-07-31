package github

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGraphQLDoer is a test double for graphQLDoer.
// fn receives the raw query, the variables, and the response pointer; it is
// responsible for populating response (typically by JSON-marshalling fake
// data into it).
type fakeGraphQLDoer struct {
	fn func(query string, variables map[string]any, response any) error
}

func (f *fakeGraphQLDoer) DoWithContext(_ context.Context, query string, variables map[string]any, response any) error {
	return f.fn(query, variables, response)
}

// populateResponse JSON-round-trips data into response, mirroring what the
// real go-gh GraphQL client does.
func populateResponse(t *testing.T, data map[string]any, response any) {
	t.Helper()
	b, err := json.Marshal(data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, response))
}

func TestPullRequests(t *testing.T) {
	doer := &fakeGraphQLDoer{
		fn: func(query string, variables map[string]any, response any) error {
			// Assert the query contains the expected alias shape.
			assert.Contains(t, query, `pr_0: pullRequest(number: 101)`)
			assert.Contains(t, query, `pr_1: pullRequest(number: 102)`)
			assert.Contains(t, query, `repository(owner: $owner, name: $name)`)
			assert.Equal(t, map[string]any{"owner": "gravitational", "name": "teleport"}, variables)

			populateResponse(t, map[string]any{
				"repository": map[string]any{
					"pr_0": map[string]any{"body": "changelog: fix a", "number": 101, "title": "Fix A", "url": "https://github.com/gravitational/teleport/pull/101"},
					"pr_1": map[string]any{"body": "changelog: fix b", "number": 102, "title": "Fix B", "url": "https://github.com/gravitational/teleport/pull/102"},
				},
			}, response)
			return nil
		},
	}

	cl := &Client{graphql: doer}
	prs, err := cl.PullRequests(context.Background(), "gravitational", "teleport", []int{101, 102})
	require.NoError(t, err)
	require.Len(t, prs, 2)
	assert.Equal(t, 101, prs[0].Number)
	assert.Equal(t, "Fix A", prs[0].Title)
	assert.Equal(t, 102, prs[1].Number)
}

func TestPullRequests_Paging(t *testing.T) {
	callCount := 0
	doer := &fakeGraphQLDoer{
		fn: func(query string, _ map[string]any, response any) error {
			callCount++
			repo := make(map[string]any)
			switch callCount {
			case 1:
				// First batch must be exactly 50 PRs.
				assert.Contains(t, query, "pr_49:", "first batch should contain pr_49")
				assert.NotContains(t, query, "pr_50:", "first batch should not contain pr_50")
				for i := 0; i < 50; i++ {
					repo[fmt.Sprintf("pr_%d", i)] = map[string]any{
						"body": "", "number": i + 1, "title": fmt.Sprintf("PR %d", i+1), "url": "",
					}
				}
			case 2:
				// Second batch has the remaining 1 PR.
				assert.Contains(t, query, "pr_0: pullRequest(number: 51)", "second batch should request PR 51")
				assert.NotContains(t, query, "pr_1:", "second batch should request only one PR")
				repo["pr_0"] = map[string]any{
					"body": "", "number": 51, "title": "PR 51", "url": "",
				}
			}
			populateResponse(t, map[string]any{"repository": repo}, response)
			return nil
		},
	}

	numbers := make([]int, 51)
	for i := range numbers {
		numbers[i] = i + 1
	}

	cl := &Client{graphql: doer}
	prs, err := cl.PullRequests(context.Background(), "org", "repo", numbers)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount, "51 PRs should require exactly 2 batches")
	require.Len(t, prs, 51)
	for i, pr := range prs {
		assert.Equal(t, i+1, pr.Number, "ordering should be preserved")
	}
}

func TestPullRequests_MissingPR(t *testing.T) {
	doer := &fakeGraphQLDoer{
		fn: func(_ string, _ map[string]any, response any) error {
			// A deleted PR comes back as null alongside a NOT_FOUND error;
			// the rest of the data is still populated.
			populateResponse(t, map[string]any{
				"repository": map[string]any{
					"pr_0": nil,
					"pr_1": map[string]any{"body": "b", "number": 202, "title": "T", "url": "u"},
					"pr_2": nil,
				},
			}, response)
			return &api.GraphQLError{Errors: []api.GraphQLErrorItem{
				{Type: "NOT_FOUND", Path: []any{"repository", "pr_0"}},
				{Type: "NOT_FOUND", Path: []any{"repository", "pr_2"}},
			}}
		},
	}

	cl := &Client{graphql: doer}
	prs, err := cl.PullRequests(context.Background(), "org", "repo", []int{201, 202, 203})
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, 202, prs[0].Number)
}

func TestPullRequests_Error(t *testing.T) {
	doer := &fakeGraphQLDoer{
		fn: func(_ string, _ map[string]any, _ any) error {
			return &api.GraphQLError{Errors: []api.GraphQLErrorItem{
				{Type: "FORBIDDEN", Path: []any{"repository", "pr_0"}},
			}}
		},
	}

	cl := &Client{graphql: doer}
	_, err := cl.PullRequests(context.Background(), "org", "repo", []int{201})
	assert.Error(t, err, "non-NOT_FOUND GraphQL errors should not be tolerated")
}

func TestPullRequests_Empty(t *testing.T) {
	doer := &fakeGraphQLDoer{
		fn: func(_ string, _ map[string]any, _ any) error {
			t.Error("DoWithContext should not be called for an empty input")
			return nil
		},
	}

	cl := &Client{graphql: doer}
	prs, err := cl.PullRequests(context.Background(), "org", "repo", nil)
	require.NoError(t, err)
	assert.Empty(t, prs)
}
