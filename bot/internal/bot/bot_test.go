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

package bot

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"

	"github.com/stretchr/testify/require"
)

// TestClassifyChanges checks that PR contents are correctly parsed for docs and
// code changes.
func TestClassifyChanges(t *testing.T) {
	tests := []struct {
		desc  string
		files []github.PullRequestFile
		docs  bool
		code  bool
	}{
		{
			desc: "code-only",
			files: []github.PullRequestFile{
				{Name: "file.go"},
				{Name: "examples/README.md"},
			},
			docs: false,
			code: true,
		},
		{
			desc: "docs-only",
			files: []github.PullRequestFile{
				{Name: "docs/docs.md"},
				{Name: "CHANGELOG.md"},
			},
			docs: true,
			code: false,
		},
		{
			desc: "code-and-code",
			files: []github.PullRequestFile{
				{Name: "file.go"},
				{Name: "docs/docs.md"},
			},
			docs: true,
			code: true,
		},
		{
			desc:  "no-docs-no-code",
			files: nil,
			docs:  false,
			code:  false,
		},
	}
	e := &env.Environment{Repository: env.TeleportRepo}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			docs, code, err := classifyChanges(e, test.files)
			require.NoError(t, err)
			require.Equal(t, docs, test.docs)
			require.Equal(t, code, test.code)
		})
	}
}

func TestXLargePRs(t *testing.T) {
	tests := []struct {
		desc  string
		files []github.PullRequestFile
		isXL  bool
	}{
		{
			desc: "single file xlarge",
			files: []github.PullRequestFile{
				{Name: "file.go", Additions: 5555},
			},
			isXL: true,
		},
		{
			desc: "single file not xlarge",
			files: []github.PullRequestFile{
				{Name: "file.go", Additions: 5, Deletions: 2},
			},
			isXL: false,
		},
		{
			desc: "multiple files xlarge",
			files: []github.PullRequestFile{
				{Name: "file.go", Additions: 502, Deletions: 2},
				{Name: "file2.go", Additions: 10000, Deletions: 2000},
			},
			isXL: true,
		},
		{
			desc: "with autogen, not xlarge",
			files: []github.PullRequestFile{
				{Name: "file.go", Additions: 502, Deletions: 2},
				{Name: "file2.pb.go", Additions: 10000, Deletions: 2000},
				{Name: "file_pb.js", Additions: 10000, Deletions: 2000},
				{Name: "file2_pb.d.ts", Additions: 10000, Deletions: 2000},
				{Name: "webassets/12345/app.js", Additions: 10000, Deletions: 2000},
				{Name: "vendor/golang.org/x/sys", Additions: 10000, Deletions: 2000},
			},
			isXL: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			require.Equal(t, test.isXL, xlargeRequiresAdminApproval(test.files))
		})
	}
}

func TestIsInternal(t *testing.T) {
	for _, test := range []struct {
		desc          string
		codeReviewers []string
		docsReviewers []string
		orgMembers    []string
		author        string
		isInternal    bool
	}{
		{
			desc:          "code reviewer",
			codeReviewers: []string{"foo", "bar"},
			docsReviewers: []string{"baz", "quux"},
			author:        "foo",
			isInternal:    true,
		},
		{
			desc:          "docs reviewer",
			codeReviewers: []string{"foo", "bar"},
			docsReviewers: []string{"baz", "quux"},
			author:        "quux",
			isInternal:    true,
		},
		{
			desc:          "org member reviewer",
			codeReviewers: []string{"foo", "bar"},
			docsReviewers: []string{"baz", "quux"},
			orgMembers:    []string{"russjones"},
			author:        "russjones",
			isInternal:    true,
		},
		{
			desc:          "rando",
			codeReviewers: []string{"foo", "bar"},
			docsReviewers: []string{"baz", "quux"},
			orgMembers:    []string{"russjones"},
			author:        "hacker",
			isInternal:    false,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			gh := &fakeGithub{orgMembers: make(map[string]struct{})}
			for _, member := range test.orgMembers {
				gh.orgMembers[member] = struct{}{}
			}
			rc := &review.Config{
				Admins:            []string{},
				CodeReviewers:     make(map[string]review.Reviewer),
				CodeReviewersOmit: map[string]bool{},
				DocsReviewers:     make(map[string]review.Reviewer),
				DocsReviewersOmit: make(map[string]bool),
			}
			for _, cr := range test.codeReviewers {
				rc.CodeReviewers[cr] = review.Reviewer{}
			}
			for _, dr := range test.docsReviewers {
				rc.DocsReviewers[dr] = review.Reviewer{}
			}

			r, err := review.New(rc)
			require.NoError(t, err)

			b := Bot{
				c: &Config{
					GitHub:      gh,
					Review:      r,
					Environment: &env.Environment{Author: test.author},
				},
			}

			internal, err := b.isInternal(context.Background())
			require.NoError(t, err)
			require.Equal(t, test.isInternal, internal)
		})
	}
}

// TestDoNotMerge verifies that PRs with do-not-merge label fail the
// reviewers check.
func TestDoNotMerge(t *testing.T) {
	gh := &fakeGithub{
		pull: github.PullRequest{
			UnsafeLabels: []string{doNotMergeLabel},
		},
	}
	bot := Bot{
		c: &Config{
			GitHub: gh,
			Environment: &env.Environment{
				Organization: "gravitational",
				Repository:   "teleport",
				Number:       42,
			},
		},
	}
	require.Error(t, bot.checkDoNotMerge(context.Background()))
}

type fakeGithub struct {
	files      []github.PullRequestFile
	pull       github.PullRequest
	reviewers  []string
	reviews    []github.Review
	orgMembers map[string]struct{}
}

func (f *fakeGithub) RequestReviewers(ctx context.Context, organization string, repository string, number int, reviewers []string) error {
	return nil
}

func (f *fakeGithub) DismissReviewers(ctx context.Context, organization string, repository string, number int, reviewers []string) error {
	return nil
}

func (f *fakeGithub) ListReviews(ctx context.Context, organization string, repository string, number int) ([]github.Review, error) {
	return f.reviews, nil
}

func (f *fakeGithub) ListReviewers(ctx context.Context, organization string, repository string, number int) ([]string, error) {
	return f.reviewers, nil
}

func (f *fakeGithub) GetPullRequest(ctx context.Context, organization string, repository string, number int) (github.PullRequest, error) {
	return f.pull, nil
}

func (f *fakeGithub) GetPullRequestWithCommits(ctx context.Context, organization string, repository string, number int) (github.PullRequest, error) {
	return f.pull, nil
}

func (f *fakeGithub) ListPullRequests(ctx context.Context, organization string, repository string, state string) ([]github.PullRequest, error) {
	return nil, nil
}

func (f *fakeGithub) ListFiles(ctx context.Context, organization string, repository string, number int) ([]github.PullRequestFile, error) {
	return f.files, nil
}

func (f *fakeGithub) AddLabels(ctx context.Context, organization string, repository string, number int, labels []string) error {
	return nil
}

func (f *fakeGithub) ListWorkflows(ctx context.Context, organization string, repository string) ([]github.Workflow, error) {
	return nil, nil
}

func (f *fakeGithub) ListWorkflowRuns(ctx context.Context, organization string, repository string, branch string, workflowID int64) ([]github.Run, error) {
	return nil, nil
}

func (f *fakeGithub) ListWorkflowJobs(ctx context.Context, organization string, repository string, runID int64) ([]github.Job, error) {
	return nil, nil
}

func (f *fakeGithub) DeleteWorkflowRun(ctx context.Context, organization string, repository string, runID int64) error {
	return nil
}

func (f *fakeGithub) IsOrgMember(ctx context.Context, user string, org string) (bool, error) {
	_, member := f.orgMembers[user]
	return member, nil
}

func (f *fakeGithub) CreateComment(ctx context.Context, organization string, repository string, number int, comment string) error {
	return nil
}

func (f *fakeGithub) ListComments(ctx context.Context, organization string, repository string, number int) ([]string, error) {
	return nil, nil
}

func (f *fakeGithub) CreatePullRequest(ctx context.Context, organization string, repository string, title string, head string, base string, body string, draft bool) (int, error) {
	return 0, nil
}
