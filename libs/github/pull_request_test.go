package github

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	go_github "github.com/google/go-github/v69/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSearchService struct {
	t *testing.T
}

func (s *testSearchService) Issues(ctx context.Context, query string, opts *go_github.SearchOptions) (*go_github.IssuesSearchResult, *go_github.Response, error) {
	f, err := os.Open(filepath.Join("testdata", "list-changelog-prs.json"))
	require.NoError(s.t, err)

	res := new(go_github.IssuesSearchResult)
	err = json.NewDecoder(f).Decode(res)
	require.NoError(s.t, err, "could not unmarshal test data")
	return res, nil, nil
}

func TestListChangelogPullRequests(t *testing.T) {
	cl := Client{
		search: &testSearchService{t: t},
	}

	prs, err := cl.ListChangelogPullRequests(context.TODO(), "test", "test", &ListChangelogPullRequestsOpts{})
	require.NoError(t, err)

	// Just a quick sanity test to ensure that things don't break
	assert.Len(t, prs, 13)
	assert.Equal(t, prs[0].URL, "https://github.com/gravitational/teleport.e/pull/4449")
	assert.Equal(t, prs[0].Number, 4449)
	assert.Equal(t, prs[0].Title, "[v16] Web UI: Validate GCP resource name, update label copy")
	assert.Equal(t, prs[0].Body, "Backports https://github.com/gravitational/teleport.e/pull/4375 and https://github.com/gravitational/teleport.e/pull/4423 to Branch/v16")
}
