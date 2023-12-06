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

	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
)

func TestFindBranches(t *testing.T) {
	branches := findBranches([]string{
		"backport/branch/v7",
		"backport/branchv8",
		"backport/master",
		"backport/foo",
		"branch/v9",
	})
	require.ElementsMatch(t, branches, []string{
		"branch/v7",
		"master",
	})
}

func TestBackport(t *testing.T) {
	buildTestBot := func(github Client) (*Bot, context.Context) {
		r, _ := review.New(&review.Config{
			CodeReviewers: map[string]review.Reviewer{"dev": review.Reviewer{
				Team: "core",
			}},
			CodeReviewersOmit: map[string]bool{},
			DocsReviewers:     map[string]review.Reviewer{},
			DocsReviewersOmit: map[string]bool{},
			Admins:            []string{},
		})

		return &Bot{
			c: &Config{
				Environment: &env.Environment{
					Organization: "foo",
					Author:       "dev",
					Repository:   "bar",
					Number:       42,
					UnsafeBase:   "branch/v8",
					UnsafeHead:   "fix",
				},
				GitHub: github,
				Review: r,
				Git:    gitDryRun,
			},
		}, context.Background()
	}

	tests := []struct {
		desc       string
		github     Client
		assertFunc require.ValueAssertionFunc
	}{
		{
			desc:       "pr without backport labels",
			github:     &fakeGithub{},
			assertFunc: require.Empty,
		},
		{
			desc: "pr with backport label, no changelog",
			github: &fakeGithub{
				pull: github.PullRequest{
					Author:       "dev",
					Repository:   "Teleport",
					Number:       42,
					UnsafeTitle:  "Best PR",
					UnsafeBody:   "This is PR body",
					UnsafeLabels: []string{"backport/branch/v7"},
				},
				jobs: []github.Job{{Name: "Job1", ID: 1}},
			},
			assertFunc: func(t require.TestingT, i interface{}, i2 ...interface{}) {
				comments, ok := i.([]github.Comment)
				require.True(t, ok)
				require.Len(t, comments, 1)
				require.Equal(t,
					`
@dev See the table below for backport results.

| Branch | Result |
|--------|--------|
| branch/v7 | [Create PR](https://github.com/foo/bar/compare/branch/v7...bot/backport-42-branch/v7?body=Backport+%2342+to+branch%2Fv7&expand=1&labels=no-changelog&title=%5Bv7%5D+Best+PR) |
`, comments[0].Body)
			},
		},
		{
			desc: "pr with backport label and with changelog",
			github: &fakeGithub{
				pull: github.PullRequest{
					Author:       "dev",
					Repository:   "Teleport",
					Number:       42,
					UnsafeTitle:  "Best PR",
					UnsafeBody:   "This is PR body\n\nchangelog: important change",
					UnsafeLabels: []string{"backport/branch/v7"},
				},
				jobs: []github.Job{{Name: "Job1", ID: 1}},
			},
			assertFunc: func(t require.TestingT, i interface{}, i2 ...interface{}) {
				comments, ok := i.([]github.Comment)
				require.True(t, ok)
				require.Len(t, comments, 1)
				require.Equal(t,
					`
@dev See the table below for backport results.

| Branch | Result |
|--------|--------|
| branch/v7 | [Create PR](https://github.com/foo/bar/compare/branch/v7...bot/backport-42-branch/v7?body=Backport+%2342+to+branch%2Fv7%0A%0Achangelog%3A+important+change%0A&expand=1&title=%5Bv7%5D+Best+PR) |
`, comments[0].Body)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b, ctx := buildTestBot(test.github)

			err := b.Backport(ctx)
			require.NoError(t, err)

			comments, _ := b.c.GitHub.ListComments(nil, "", "", 0)
			test.assertFunc(t, comments)
		})
	}
}
