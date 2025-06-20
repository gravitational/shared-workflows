/*
Copyright 2021 Gravitational, Inc.

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

package review

import (
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
)

// TestIsInternal checks if docs and code reviewers show up as internal.
func TestIsInternal(t *testing.T) {
	tests := []struct {
		desc        string
		assignments *Assignments
		author      string
		expect      bool
	}{
		{
			desc: "core-is-internal",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CodeReviewersOmit: map[string]bool{},
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"5": {Owner: true},
						"6": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			author: "1",
			expect: true,
		},
		{
			desc: "cloud-is-internal",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CloudReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CodeReviewersOmit: map[string]bool{},
					DocsReviewers:     map[string]Reviewer{},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			author: "1",
			expect: true,
		},
		{
			desc: "docs-is-internal",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CodeReviewersOmit: map[string]bool{},
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"5": {Owner: true},
						"6": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			author: "5",
			expect: true,
		},
		{
			desc: "other-is-not-internal",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CodeReviewersOmit: map[string]bool{},
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"5": {Owner: true},
						"6": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			author: "7",
			expect: false,
		},
		{
			desc: "dependabot is internal",
			assignments: &Assignments{
				c: &Config{},
			},
			author: Dependabot,
			expect: true,
		},
		{
			desc: "dependabot batcher is internal",
			assignments: &Assignments{
				c: &Config{},
			},
			author: DependabotBatcher,
			expect: true,
		},
		{
			desc: "renovate public is internal",
			assignments: &Assignments{
				c: &Config{},
			},
			author: RenovateBotPublic,
			expect: true,
		},
		{
			desc: "renovate private is internal",
			assignments: &Assignments{
				c: &Config{},
			},
			author: RenovateBotPrivate,
			expect: true,
		},
		{
			desc: "post-release bot is internal",
			assignments: &Assignments{
				c: &Config{},
			},
			author: PostReleaseBot,
			expect: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			got := test.assignments.IsInternal(test.author)
			require.Equal(t, test.expect, got)
		})
	}
}

// TestGetCodeReviewers checks internal code review assignments.
func TestGetCodeReviewerSets(t *testing.T) {
	tests := []struct {
		desc        string
		assignments *Assignments
		author      string
		repository  string
		setA        []string
		setB        []string
	}{
		{
			desc: "skip-self-assign",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CloudReviewers:    map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport",
			author:     "1",
			setA:       []string{"2"},
			setB:       []string{"3", "4"},
		},
		{
			desc: "skip-omitted-user",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
						"5": {Owner: false},
					},
					CloudReviewers: map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{
						"3": true,
					},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport",
			author:     "5",
			setA:       []string{"1", "2"},
			setB:       []string{"4"},
		},
		{
			desc: "internal-gets-defaults",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: false},
						"4": {Owner: false},
					},
					CloudReviewers:    map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport",
			author:     "5",
			setA:       []string{"1"},
			setB:       []string{"2"},
		},
		{
			desc: "cloud-gets-core-reviewers",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: true},
						"4": {Owner: false},
						"5": {Owner: false},
						"6": {Owner: false},
					},
					CloudReviewers: map[string]Reviewer{
						"8": {Owner: false},
						"9": {Owner: false},
					},
					CodeReviewersOmit: map[string]bool{
						"6": true,
					},
					// Docs.
					DocsReviewers:     map[string]Reviewer{},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport",
			author:     "8",
			setA:       []string{"1", "2", "3"},
			setB:       []string{"4", "5"},
		},
		{
			desc: "normal",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: true},
						"4": {Owner: false},
						"5": {Owner: false},
						"6": {Owner: false},
					},
					CloudReviewers: map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{
						"6": true,
					},
					// Docs.
					DocsReviewers:     map[string]Reviewer{},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport",
			author:     "4",
			setA:       []string{"1", "2", "3"},
			setB:       []string{"5"},
		},
		{
			desc: "normal (teleport.e)",
			assignments: &Assignments{
				c: &Config{
					// Code.
					CoreReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
						"3": {Owner: true},
						"4": {Owner: false},
						"5": {Owner: false},
						"6": {Owner: false},
					},
					CloudReviewers: map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{
						"6": true,
					},
					// Docs.
					DocsReviewers:     map[string]Reviewer{},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"1",
						"2",
					},
				},
			},
			repository: "teleport.e",
			author:     "4",
			setA:       []string{"1", "2", "3"},
			setB:       []string{"5"},
		},
		{
			desc: "docs reviewers submitting code changes are treated as internal authors",
			assignments: &Assignments{
				c: &Config{
					CoreReviewers: map[string]Reviewer{
						"code-1": {Owner: true},
						"code-2": {Owner: false},
					},
					CloudReviewers: map[string]Reviewer{},
					DocsReviewers: map[string]Reviewer{
						"docs-1": {Owner: true},
					},
					Admins: []string{"code-1", "code-2"},
				},
			},
			repository: "teleport",
			author:     "docs-1",
			setA:       []string{"code-1"},
			setB:       []string{"code-2"},
		},
		{
			desc: "admins can be omitted from code reviews",
			assignments: &Assignments{
				c: &Config{
					CoreReviewers: map[string]Reviewer{
						"code-1": {Owner: true},
						"code-2": {Owner: false},
					},
					Admins: []string{
						"code-1",
						"code-2",
						"code-3",
					},
					CloudReviewers: map[string]Reviewer{},
					CodeReviewersOmit: map[string]bool{
						"code-1": true,
					},
				},
			},
			repository: "teleport",
			author:     "external-1",
			setA:       []string{"code-2"},
			setB:       []string{"code-3"},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			// We just want to validate that this test gets to the point where it's
			// checking for approvals. This means the bot did not reject the PR
			// as an external author.

			e := &env.Environment{
				Repository: test.repository,
				Author:     test.author,
			}
			changes := env.Changes{ApproverCount: env.DefaultApproverCount}
			require.ErrorContains(t, test.assignments.checkInternalReviews(e, changes, nil, nil),
				"at least one approval required from each set")

			setA, setB := test.assignments.getCodeReviewerSets(e)
			require.ElementsMatch(t, setA, test.setA)
			require.ElementsMatch(t, setB, test.setB)
		})
	}
}

// TestOmitReviewersCanApprove checks the scenario in which non-admin approver is on the `Config.CodeReviewersOmit` list.
// In this case we won't be asking for their review (checked elsewhere), but we do want to honor their review is present (this test).
func TestOmitReviewersCanApprove(t *testing.T) {
	assignments := &Assignments{
		c: &Config{
			CoreReviewers: map[string]Reviewer{
				"code-1": {Owner: true},
				"code-2": {Owner: false},
			},
			Admins:         []string{},
			CloudReviewers: map[string]Reviewer{},
			CodeReviewersOmit: map[string]bool{
				"code-1": true,
				"code-2": true,
			},
		},
	}

	e := &env.Environment{
		Repository: "",
		Author:     "",
	}

	changes := env.Changes{ApproverCount: env.DefaultApproverCount}

	err := assignments.checkInternalReviews(e, changes, nil, nil)

	require.ErrorContains(t, err, "at least one approval required from each set [code-1] [code-2]")
}

// TestGetDocsReviewers checks internal docs review assignments.
func TestGetDocsReviewers(t *testing.T) {
	tests := []struct {
		desc        string
		assignments *Assignments
		author      string
		reviewers   []string
		files       []github.PullRequestFile
	}{
		{
			desc: "skip-self-assign",
			assignments: &Assignments{
				c: &Config{
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"3",
						"4",
					},
				},
			},
			author:    "1",
			reviewers: []string{"2"},
		},
		{
			desc: "skip-self-assign-with-omit",
			assignments: &Assignments{
				c: &Config{
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{
						"2": true,
					},
					// Admins.
					Admins: []string{
						"3",
						"4",
					},
				},
			},
			author:    "1",
			reviewers: []string{"3", "4"},
		},
		{
			desc: "normal",
			assignments: &Assignments{
				c: &Config{
					// Docs.
					DocsReviewers: map[string]Reviewer{
						"1": {Owner: true},
						"2": {Owner: true},
					},
					DocsReviewersOmit: map[string]bool{},
					// Admins.
					Admins: []string{
						"3",
						"4",
					},
				},
			},
			author:    "3",
			reviewers: []string{"1", "2"},
		},
		{
			desc: "preferred code reviewer for docs page",
			assignments: &Assignments{
				c: &Config{
					DocsReviewers: map[string]Reviewer{
						"1": {Owner: true},
					},
					CoreReviewers: map[string]Reviewer{
						"2": {Owner: true, PreferredReviewerFor: []string{"docs/pages/server-access"}},
						"3": {Owner: true},
					},
				},
			},
			author: "4",
			files: []github.PullRequestFile{
				{Name: "docs/pages/server-access/get-started.mdx"},
			},
			reviewers: []string{"1", "2"},
		},
		{
			desc: "preferred code reviewer for docs page with duplicate code reviewers",
			assignments: &Assignments{
				c: &Config{
					CoreReviewers: map[string]Reviewer{
						"2": {Owner: true, PreferredReviewerFor: []string{"server-access", "database-access"}},
						"3": {Owner: true, PreferredReviewerFor: []string{"server-access", "database-access"}},
					},
				},
			},
			author: "4",
			files: []github.PullRequestFile{
				{Name: "server-access/get-started.mdx"},
				{Name: "database-access/get-started.mdx"},
			},
			reviewers: []string{"2", "3"},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			e := &env.Environment{
				Repository: env.TeleportRepo,
				Author:     test.author,
			}

			reviewers := test.assignments.getDocsReviewers(e, test.files)
			require.ElementsMatch(t, reviewers, test.reviewers)
		})
	}
}

// TestCheckExternal checks external reviews.
func TestCheckExternal(t *testing.T) {
	r := &Assignments{
		c: &Config{
			// Code.
			CoreReviewers: map[string]Reviewer{
				"1": {Owner: true},
				"2": {Owner: true},
				"3": {Owner: true},
				"4": {Owner: false},
				"5": {Owner: false},
				"6": {Owner: false},
			},
			CodeReviewersOmit: map[string]bool{
				"3": true,
			},
			// Default.
			Admins: []string{
				"1",
				"2",
				"3",
			},
		},
	}
	tests := []struct {
		desc    string
		author  string
		reviews []github.Review
		result  bool
	}{
		{
			desc:    "no-reviews-fail",
			author:  "5",
			reviews: []github.Review{},
			result:  false,
		},
		{
			desc:   "two-non-admin-reviews-fail",
			author: "5",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			result: false,
		},
		{
			desc:   "one-admin-reviews-fail",
			author: "5",
			reviews: []github.Review{
				{Author: "1", State: Approved},
				{Author: "4", State: Approved},
			},
			result: false,
		},
		{
			desc:   "two-admin-reviews-one-denied-success",
			author: "5",
			reviews: []github.Review{
				{Author: "1", State: ChangesRequested},
				{Author: "2", State: Approved},
			},
			result: false,
		},
		{
			desc:   "two-admin-reviews-success",
			author: "5",
			reviews: []github.Review{
				{Author: "1", State: Approved},
				{Author: "2", State: Approved},
			},
			result: true,
		},
		{
			desc:   "two-admin-one-non-reviewer-success",
			author: "5",
			reviews: []github.Review{
				{Author: "1", State: Approved},
				{Author: "3", State: Approved},
			},
			result: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			err := r.CheckExternal(test.author, test.reviews)
			if test.result {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestCheckInternal checks internal reviews.
func TestCheckInternal(t *testing.T) {
	r := &Assignments{
		c: &Config{
			// Code.
			CoreReviewers: map[string]Reviewer{
				"1":  {Owner: true},
				"2":  {Owner: true},
				"3":  {Owner: true},
				"9":  {Owner: true},
				"4":  {Owner: false},
				"5":  {Owner: false},
				"6":  {Owner: false},
				"14": {Owner: true, PreferredReviewerFor: []string{"docs/pages/server-access"}},
			},
			CloudReviewers: map[string]Reviewer{
				"10": {Owner: false},
				"11": {Owner: false},
				"12": {Owner: false},
				"13": {Owner: true},
			},
			// Docs.
			DocsReviewers: map[string]Reviewer{
				"7": {Owner: true},
				"8": {Owner: false},
			},
			DocsReviewersOmit: map[string]bool{},
			CodeReviewersOmit: map[string]bool{},
			ReleaseReviewers: []string{
				"3",
				"4",
			},
			// Default.
			Admins: []string{
				"1",
				"2",
			},
		},
	}
	tests := []struct {
		desc           string
		author         string
		repository     string
		reviews        []github.Review
		docs           bool
		code           bool
		large          bool
		release        bool
		singleApproval bool
		result         bool
		files          []github.PullRequestFile
	}{
		{
			desc:       "no-reviews-fail",
			repository: "teleport",
			author:     "4",
			reviews:    []github.Review{},
			result:     false,
		},
		{
			desc:       "docs-only-no-reviews-fail",
			repository: "teleport",
			author:     "4",
			reviews:    []github.Review{},
			docs:       true,
			code:       false,
			result:     false,
		},
		{
			desc:       "docs-only-non-docs-approval-fail",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
			},
			docs:   true,
			code:   false,
			result: false,
		},
		{
			desc:       "docs-only-two-approval-success",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "1", State: Approved},
				{Author: "8", State: Approved},
			},
			docs:   true,
			code:   false,
			result: true,
		},
		{
			desc:       "docs-only-docs-approval-fail",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "7", State: Approved},
			},
			docs:   true,
			code:   false,
			result: false,
		},
		{
			desc:       "code-only-no-reviews-fail",
			repository: "teleport",
			author:     "4",
			reviews:    []github.Review{},
			docs:       false,
			code:       true,
			result:     false,
		},
		{
			desc:       "code-only-one-approval-fail",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "code-only-two-approval-setb-fail",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "5", State: Approved},
				{Author: "6", State: Approved},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "code-only-one-changes-fail",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: ChangesRequested},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "code-only-large-pr-requires-admin-fails",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			docs:   false,
			code:   true,
			large:  true,
			result: false,
		},
		{
			desc:       "code-only-large-pr-has-admin-succeeds",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "1", State: Approved},
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			docs:   false,
			code:   true,
			large:  true,
			result: true,
		},
		{
			desc:       "code-only-two-approvals-success",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "docs-and-code-only-docs-approval-fail",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "7", State: Approved},
			},
			docs:   true,
			code:   true,
			result: false,
		},
		{
			desc:       "docs-and-code-only-code-approval-fail",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			docs: true,
			code: true,
			// docs reviewers count towards required approvals, but are not required
			result: true,
		},
		{
			desc:       "docs-and-code-docs-and-code-approval-success",
			repository: "teleport",
			author:     "6",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
				{Author: "7", State: Approved},
			},
			docs:   true,
			code:   true,
			result: true,
		},
		{
			desc:       "code-only-internal-on-approval-failure",
			repository: "teleport",
			author:     "8",
			reviews: []github.Review{
				{Author: "3", State: Approved},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "code-only-internal-code-approval-success",
			repository: "teleport",
			author:     "8",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "4", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "code-only-internal-two-code-owner-approval-success",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "9", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "code-only-changes-requested-after-approval-failure",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "9", State: Approved},
				{Author: "9", State: ChangesRequested},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "code-only-comment-after-approval-success",
			repository: "teleport",
			author:     "4",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "9", State: Approved},
				{Author: "9", State: Commented},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "cloud-with-self-approval-failure",
			repository: "teleport",
			author:     "10",
			reviews: []github.Review{
				{Author: "11", State: Approved},
				{Author: "12", State: Approved},
			},
			docs:   false,
			code:   true,
			result: false,
		},
		{
			desc:       "cloud-with-core-approval-success",
			repository: "teleport",
			author:     "10",
			reviews: []github.Review{
				{Author: "3", State: Approved},
				{Author: "9", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "cloud-only-two-approvals-success",
			repository: "cloud",
			author:     "10",
			reviews: []github.Review{
				{Author: "12", State: Approved},
				{Author: "13", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "core-with-cloud-approval-success",
			repository: "cloud",
			author:     "1",
			reviews: []github.Review{
				{Author: "12", State: Approved},
				{Author: "13", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "cloud-code-only-internal-code-approval-success",
			repository: "cloud",
			author:     "8",
			reviews: []github.Review{
				{Author: "12", State: Approved},
				{Author: "13", State: Approved},
			},
			docs:   false,
			code:   true,
			result: true,
		},
		{
			desc:       "core-dependabot-code-not-approved-failure",
			repository: "teleport",
			author:     Dependabot,
			code:       true,
			result:     false,
		},
		{
			desc:       "core-dependabot-code-approval-success",
			repository: "teleport",
			author:     Dependabot,
			reviews: []github.Review{
				{Author: "3", State: Approved}, // owner (not admin)
				{Author: "4", State: Approved}, // not owner
			},
			code:   true,
			result: true,
		},
		{
			desc:       "core-dependabot-batcher-code-not-approved-failure",
			repository: "teleport",
			author:     DependabotBatcher,
			code:       true,
			result:     false,
		},
		{
			desc:       "core-dependabot-batcher-code-approval-success",
			repository: "teleport",
			author:     DependabotBatcher,
			reviews: []github.Review{
				{Author: "3", State: Approved}, // owner (not admin)
				{Author: "4", State: Approved}, // not owner
			},
			code:   true,
			result: true,
		},
		{
			desc:       "release-pr-fail",
			repository: "teleport",
			author:     "1",
			reviews: []github.Review{
				{Author: "5", State: Approved},
			},
			release: true,
			result:  false,
		},
		{
			desc:       "release-pr-success",
			repository: "teleport",
			author:     "1",
			reviews: []github.Review{
				{Author: "3", State: Approved},
			},
			release: true,
			result:  true,
		},
		{
			desc:       "docs-with-preferred-code-reviewer",
			repository: "teleport",
			author:     "1",
			reviews: []github.Review{
				{Author: "14", State: Approved},
			},
			files: []github.PullRequestFile{
				{
					Name: "docs/pages/server-access/get-started.mdx",
				},
			},
			docs:    true,
			code:    false,
			release: false,
			large:   false,
			result:  true,
		},
		{
			desc:       "docs-with-non-preferred-code-reviewer",
			repository: "teleport",
			author:     "1",
			reviews: []github.Review{
				{Author: "3", State: Approved},
			},
			files: []github.PullRequestFile{
				{
					Name: "docs/pages/server-access/get-started.mdx",
				},
			},
			docs:    true,
			code:    false,
			release: false,
			large:   false,
			result:  false,
		},
		{
			desc:       "cloud-single-approval-setA-success",
			repository: "cloud",
			author:     Dependabot,
			reviews: []github.Review{
				{Author: "12", State: Approved},
			},
			docs:           false,
			code:           true,
			singleApproval: true,
			result:         true,
		},
		{
			desc:       "cloud-single-approval-setB-success",
			repository: "cloud",
			author:     Dependabot,
			reviews: []github.Review{
				{Author: "13", State: Approved},
			},
			docs:           false,
			code:           true,
			singleApproval: true,
			result:         true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			e := &env.Environment{
				Repository: test.repository,
				Author:     test.author,
			}

			changes := env.Changes{
				Docs:          test.docs,
				Code:          test.code,
				Large:         test.large,
				Release:       test.release,
				ApproverCount: env.DefaultApproverCount,
			}

			if test.singleApproval {
				changes.ApproverCount = 1
			}

			err := r.CheckInternal(e, test.reviews, changes, test.files)
			if test.result {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestCheckInternalV18 ensures that admin approval is required for v18 backports.
// TODO(r0mant): Remove after 18.0.0 release.
func TestCheckInternalV18(t *testing.T) {
	r := &Assignments{
		c: &Config{
			CoreReviewers: map[string]Reviewer{
				"1": {Owner: true},
				"2": {Owner: true},
				"3": {Owner: true},
				"4": {Owner: false},
				"5": {Owner: false},
			},
			Admins: []string{
				"1",
				"2",
			},
		},
	}
	tests := []struct {
		desc       string
		author     string
		repository string
		branch     string
		reviews    []github.Review
		result     bool
	}{
		// PR to branch/v18, no admin approvals, should fail
		{
			desc:       "v18-not-admin-failure",
			repository: "teleport",
			author:     "5",
			branch:     branchV18,
			reviews: []github.Review{
				{Author: "3", State: Approved}, // not admin
				{Author: "4", State: Approved}, // not admin
			},
			result: false,
		},
		// PR to branch/v18, one admin approval, should succeed
		{
			desc:       "v18-admin-success",
			repository: "teleport",
			author:     "5",
			branch:     branchV18,
			reviews: []github.Review{
				{Author: "1", State: Approved}, // admin
				{Author: "4", State: Approved}, // not admin
			},
			result: true,
		},
		// PR to some other branch, no admin approvals, should succeed
		{
			desc:       "not-v18-not-admin-success",
			repository: "teleport",
			author:     "5",
			branch:     "main",
			reviews: []github.Review{
				{Author: "3", State: Approved}, // not admin
				{Author: "4", State: Approved}, // not admin
			},
			result: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			e := &env.Environment{
				Repository: test.repository,
				Author:     test.author,
				UnsafeBase: test.branch,
			}

			changes := env.Changes{
				Code:          true,
				ApproverCount: env.DefaultApproverCount,
			}

			err := r.CheckInternal(e, test.reviews, changes, nil)
			if test.result {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestFromString tests if configuration is correctly read in from a string.
func TestFromString(t *testing.T) {
	e := &env.Environment{Repository: env.TeleportRepo}
	r, err := FromString(e, reviewers)
	require.NoError(t, err)

	require.EqualValues(t, r.c.CoreReviewers, map[string]Reviewer{
		"1": {
			Owner: true,
		},
		"2": {
			Owner: false,
		},
	})
	require.EqualValues(t, r.c.CodeReviewersOmit, map[string]bool{
		"3": true,
	})
	require.EqualValues(t, r.c.DocsReviewers, map[string]Reviewer{
		"4": {
			Owner: true,
		},
		"5": {
			Owner: false,
		},
	})
	require.EqualValues(t, r.c.DocsReviewersOmit, map[string]bool{
		"6": true,
	})
	require.EqualValues(t, r.c.Admins, []string{
		"7",
		"8",
	})
}

type randStatic struct{}

func (r *randStatic) Intn(n int) int {
	return 0
}

func TestGetCodeReviewers(t *testing.T) {
	assignments := &Assignments{
		c: &Config{
			Rand: &randStatic{},
			CoreReviewers: map[string]Reviewer{
				"1": {Owner: true, PreferredReviewerFor: []string{"lib/srv/db", "lib/srv/app"}, PreferredOnly: true},
				"2": {Owner: true, PreferredReviewerFor: []string{"lib/srv/db", "lib/alpn"}},
				"3": {Owner: true},
				"4": {Owner: false, PreferredReviewerFor: []string{"lib/srv/app"}, PreferredOnly: true},
				"5": {Owner: false, PreferredReviewerFor: []string{"lib/srv/db"}},
				"6": {Owner: false},
			},
			Admins: []string{
				"100",
				"200",
			},
		},
	}

	tests := []struct {
		description string
		author      string
		files       []github.PullRequestFile
		expected    []string
	}{
		{
			description: "both reviewers are preferred",
			author:      "3",
			files: []github.PullRequestFile{
				{Name: "lib/srv/db/engine.go"},
			},
			expected: []string{"1", "5"},
		},
		{
			description: "one of the reviewers is preferred",
			author:      "3",
			files: []github.PullRequestFile{
				{Name: "lib/alpn/proxy.go"},
			},
			expected: []string{"2", "5"},
		},
		{
			description: "preferred reviewers for different files",
			author:      "3",
			files: []github.PullRequestFile{
				{Name: "lib/alpn/proxy.go"},
				{Name: "lib/srv/app.go"},
			},
			expected: []string{"1", "2", "4"},
		},
		{
			description: "no preferred reviewers",
			author:      "3",
			files: []github.PullRequestFile{
				{Name: "lib/service/service.go"},
			},
			expected: []string{"2", "5"},
		},
		{
			description: "covered paths: don't add new or duplicate reviewers for paths already covered",
			author:      "3",
			files: []github.PullRequestFile{
				{Name: "lib/srv/app.go"},
				{Name: "lib/srv/db/engine.go"},
			},
			expected: []string{"1", "4", "5"},
		},
		{
			description: "admins",
			author:      "999",
			files: []github.PullRequestFile{
				{Name: "lib/srv/app.go"},
				{Name: "lib/srv/db/engine.go"},
			},
			expected: []string{"100", "200"},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			e := &env.Environment{
				Repository: env.TeleportRepo,
				Author:     test.author,
			}
			actual := assignments.getCodeReviewers(e, test.files)
			sort.Strings(actual)
			require.ElementsMatch(t, test.expected, actual)
		})
	}
}

func TestSingleApproverAuthors(t *testing.T) {
	name := func(authors []string, i int) string {
		if i == -1 {
			return ""
		}
		return authors[i]
	}
	for repo, authors := range singleApproverAuthors {
		i := slices.IndexFunc(authors, func(author string) bool {
			return !isAllowedRobot(author)
		})
		require.Equal(t, -1, i, "%q is not allowed to be a single approver author in the %q repository (only bots)", name(authors, i), repo)
	}
}

const reviewers = `
{
	"coreReviewers": {
		"1": {
			"owner": true
		},
		"2": {
			"owner": false
		}
	},
	"cloudReviewers": {
		"1": {
			"owner": true
		},
		"2": {
			"owner": false
		}
	},
	"codeReviewersOmit": {
		"3": true
    },
	"docsReviewers": {
		"4": {
			"owner": true
		},
		"5": {
			"owner": false
		}
	},
	"docsReviewersOmit": {
		"6": true
    },
	"admins": [
		"7",
		"8"
	]
}
`
