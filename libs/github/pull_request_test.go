package github

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	go_github "github.com/google/go-github/v63/github"
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
	assert.Len(t, prs, 13)
	assert.Equal(t, prs[0].URL, "https://github.com/gravitational/teleport.e/pull/4449")
}
