/*
Copyright 2022 Gravitational, Inc.

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

package bot

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"

	"github.com/stretchr/testify/require"
)

// TestBackportReviewers checks if backport reviewers are correctly assigned.
func TestBackportReviewers(t *testing.T) {
	r, err := review.New(&review.Config{
		CoreReviewers:     map[string]review.Reviewer{},
		CloudReviewers:    map[string]review.Reviewer{},
		CodeReviewersOmit: map[string]bool{},
		DocsReviewers:     map[string]review.Reviewer{},
		DocsReviewersOmit: map[string]bool{},
		Admins:            []string{},
	})
	require.NoError(t, err)

	const (
		botUser           = "espa[bot]lini"
		luke              = "luke"
		anakin            = "anakin"
		palpatine         = "palpatine"
		noLongerInOrgUser = "reviewer-no-longer-in-org-user"
		author            = "someUserForTest"
	)

	tests := []struct {
		desc       string
		pull       github.PullRequest
		reviewers  []string
		reviews    []github.Review
		orgMembers []string
		err        bool
		expected   []string
	}{
		{
			desc:      "backport-original-pr-number-approved",
			pull:      pr("Backport #0 to branch/v8", ""),
			reviewers: []string{luke, botUser},
			reviews: []github.Review{
				{Author: anakin, State: review.Approved},
			},
			orgMembers: []string{author, luke, anakin},
			err:        false,
			expected:   []string{luke, anakin},
		},
		{
			desc:      "backport-original-url-approved",
			pull:      pr("Fixed an issue", "https://github.com/gravitational/teleport/pull/0"),
			reviewers: []string{luke},
			reviews: []github.Review{
				{Author: anakin, State: review.Approved},
			},
			orgMembers: []string{author, luke, anakin},
			err:        false,
			expected:   []string{luke, anakin},
		},
		{
			desc:      "backport-reviewed-by-bot",
			pull:      pr("Fixed an issue", "https://github.com/gravitational/teleport/pull/0"),
			reviewers: []string{luke},
			reviews: []github.Review{
				{Author: anakin, State: review.Approved},
				{Author: botUser, State: review.Approved},
			},
			orgMembers: []string{author, luke, anakin},
			err:        false,
			expected:   []string{luke, anakin},
		},
		{
			desc:      "backport-multiple-reviews",
			pull:      pr("Fixed an issue", ""),
			reviewers: []string{"3"},
			reviews: []github.Review{
				{Author: anakin, State: review.Commented},
				{Author: anakin, State: review.ChangesRequested},
				{Author: anakin, State: review.Approved},
				{Author: palpatine, State: review.Approved},
			},
			orgMembers: []string{author, luke, anakin},
			err:        true,
			expected:   []string{},
		},
		{
			desc:       "backport-original-not-found",
			pull:       pr("Fixed feature", ""),
			orgMembers: []string{anakin, author},
			reviewers:  []string{luke},
			reviews: []github.Review{
				{Author: anakin, State: review.Approved},
			},
			err:      true,
			expected: []string{},
		},
		{
			desc:       "backport-reviewer-left-org",
			pull:       pr("Backport #0 to branch/v8", ""),
			orgMembers: []string{luke, anakin, author},
			reviewers:  []string{luke, anakin, noLongerInOrgUser},
			reviews: []github.Review{
				{Author: luke, State: review.Approved},
				{Author: anakin, State: review.Approved},
				{Author: noLongerInOrgUser, State: review.Approved},
			},
			err:      false,
			expected: []string{luke, anakin},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			githubUserIDToIsMember := make(map[string]struct{})
			for _, member := range test.orgMembers {
				githubUserIDToIsMember[member] = struct{}{}
			}
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Organization: "foo",
						Author:       "9",
						Repository:   "bar",
						Number:       0,
						UnsafeBase:   "branch/v8",
						UnsafeHead:   "fix",
					},
					Review: r,
					GitHub: &fakeGithub{
						pull:       test.pull,
						reviewers:  test.reviewers,
						reviews:    test.reviews,
						orgMembers: githubUserIDToIsMember,
					},
				},
			}
			reviewers, err := b.backportReviewers(context.Background())
			require.Equal(t, err != nil, test.err)
			require.ElementsMatch(t, reviewers, test.expected)
		})
	}
}

func pr(title, body string) github.PullRequest {
	return github.PullRequest{
		Author:     "baz",
		Repository: "repositoryNameForTest",
		UnsafeHead: github.Branch{
			Ref: "baz/fix",
		},
		UnsafeTitle: title,
		UnsafeBody:  body,
		Fork:        false,
	}
}
