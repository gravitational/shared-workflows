package bot

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
	"github.com/stretchr/testify/require"
)

func TestSelectRFDReviewTeam(t *testing.T) {
	r, err := review.New(&review.Config{
		CoreReviewers:     map[string]review.Reviewer{"both": {}, "core": {}},
		CloudReviewers:    map[string]review.Reviewer{"both": {}, "cloud": {}},
		CodeReviewersOmit: map[string]bool{},
		DocsReviewers:     map[string]review.Reviewer{},
		DocsReviewersOmit: map[string]bool{},
		Admins:            []string{"admin"},
	})
	require.NoError(t, err)

	for _, test := range []struct {
		name      string
		author    string
		labels    []string
		wantTeam  string
		wantError string
	}{
		{"no label", "both", nil, "", ""},
		{"core label", "both", []string{rfdCoreReviewLabel}, env.CoreTeam, ""},
		{"cloud label", "both", []string{rfdCloudReviewLabel}, env.CloudTeam, ""},
		{"matching single pool", "core", []string{rfdCoreReviewLabel}, env.CoreTeam, ""},
		{"conflicting labels", "both", []string{rfdCoreReviewLabel, rfdCloudReviewLabel}, "", "cannot have both"},
		{"wrong pool", "core", []string{rfdCloudReviewLabel}, env.CloudTeam, "not in the Cloud reviewer pool"},
		{"no membership", "other", []string{rfdCoreReviewLabel}, env.CoreTeam, "not in the Core reviewer pool"},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := &Bot{c: &Config{
				Environment: &env.Environment{Repository: env.RFDRepo, Author: test.author},
				GitHub:      &fakeGithub{pull: github.PullRequest{UnsafeLabels: test.labels}},
				Review:      r,
			}}
			err := b.selectRFDReviewTeam(context.Background())
			if test.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantError)
			}
			require.Equal(t, test.wantTeam, b.c.Environment.RFDReviewTeam)
		})
	}
}
