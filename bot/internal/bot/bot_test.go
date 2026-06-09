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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
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
	c := &Config{Environment: &env.Environment{Repository: env.TeleportRepo}}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			changes := classifyChanges(c, test.files)
			require.Equal(t, changes.Docs, test.docs)
			require.Equal(t, changes.Code, test.code)
		})
	}
}

func TestIsReleasePR(t *testing.T) {
	tests := []struct {
		desc      string
		env       *env.Environment
		files     []github.PullRequestFile
		isRelease bool
	}{
		{
			desc: "release-pr",
			env: &env.Environment{
				UnsafeHead: "release/14.0.0",
			},
			files: []github.PullRequestFile{
				{Name: "CHANGELOG.md"},
				{Name: "Makefile"},
				{Name: "version.go"},
				{Name: "api/version.go"},
				{Name: "integrations/kube-agent-updater/version.go"},
			},
			isRelease: true,
		},
		{
			desc: "non-release-pr-invalid-branch",
			env: &env.Environment{
				UnsafeHead: "roman/14.0.0",
			},
			files: []github.PullRequestFile{
				{Name: "CHANGELOG.md"},
				{Name: "Makefile"},
				{Name: "version.go"},
				{Name: "api/version.go"},
				{Name: "integrations/kube-agent-updater/version.go"},
			},
			isRelease: false,
		},
		{
			desc: "non-release-pr-missing-release-file",
			env: &env.Environment{
				UnsafeHead: "release/14.0.0",
			},
			files: []github.PullRequestFile{
				{Name: "CHANGELOG.md"},
				{Name: "Makefile"},
				{Name: "version.go"},
				{Name: "api/version.go"},
			},
			isRelease: false,
		},
		{
			desc: "non-release-pr-invalid-files",
			env: &env.Environment{
				UnsafeHead: "release/14.0.0",
			},
			files: []github.PullRequestFile{
				{Name: "lib/auth/auth.go"},
			},
			isRelease: false,
		},
		{
			desc: "non-release-pr-extra-source-files",
			env: &env.Environment{
				UnsafeHead: "release/14.0.0",
			},
			files: []github.PullRequestFile{
				{Name: "CHANGELOG.md"},
				{Name: "Makefile"},
				{Name: "version.go"},
				{Name: "api/version.go"},
				{Name: "integrations/kube-agent-updater/version.go"},
				{Name: "lib/auth/auth.go"},
			},
			isRelease: false,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			require.Equal(t, test.isRelease, isReleasePR(test.env, test.files))
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
				{Name: "testdata/tests.json", Additions: 10000, Deletions: 2000},
				{Name: "vendor/golang.org/x/sys", Additions: 10000, Deletions: 2000},
				{Name: "gen/preset-roles.json", Additions: 1000, Deletions: 0},
				{Name: "examples/chart/teleport-cluster/charts/teleport-operator/operator-crds/resources.teleport.dev_rolesv8.yaml", Additions: 1500, Deletions: 0},
				{Name: "integrations/operator/crdgen/testdata/golden/resources.teleport.dev_rolesv8.yaml", Additions: 1500, Deletions: 0},
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
				CoreReviewers:     make(map[string]review.Reviewer),
				CloudReviewers:    make(map[string]review.Reviewer),
				CodeReviewersOmit: map[string]bool{},
				DocsReviewers:     make(map[string]review.Reviewer),
				DocsReviewersOmit: make(map[string]bool),
			}
			for _, cr := range test.codeReviewers {
				rc.CoreReviewers[cr] = review.Reviewer{}
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

func TestApproverCount(t *testing.T) {
	cases := []struct {
		desc    string
		author  string
		authors []string
		paths   []string
		files   []github.PullRequestFile
		expect  int
	}{
		{
			desc:   "no paths or files",
			expect: env.DefaultApproverCount,
		},
		{
			desc: "no paths",
			files: []github.PullRequestFile{
				{Name: "lib/default.go"},
			},
			expect: env.DefaultApproverCount,
		},
		{
			desc:   "no files",
			paths:  []string{"single/approver"},
			expect: env.DefaultApproverCount,
		},
		{
			desc: "no files matching paths",
			files: []github.PullRequestFile{
				{Name: "lib/default.go"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"single/approver"},
			expect: env.DefaultApproverCount,
		},
		{
			desc: "one file matching path",
			files: []github.PullRequestFile{
				{Name: "lib/default.go"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"lib/db.go"},
			expect: env.DefaultApproverCount,
		},
		{
			desc: "all files matching path",
			files: []github.PullRequestFile{
				{Name: "lib/default.go"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"lib"},
			expect: 1,
		},
		{
			desc: "all files match wildcard",
			files: []github.PullRequestFile{
				{Name: "src/package/values.yaml"},
				{Name: "src/package2/values.yaml"},
			},
			paths:  []string{"src/*/values.yaml"},
			expect: 1,
		},
		{
			desc: "all files match multiple wildcard",
			files: []github.PullRequestFile{
				{Name: "src/package/values.yaml"},
				{Name: "docs/README.md"},
			},
			paths:  []string{"src/*/values.yaml", "*/*.md"},
			expect: 1,
		},
		{
			desc: "all files match wildcard or path",
			files: []github.PullRequestFile{
				{Name: "src/package/values.yaml"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"lib", "src/*/values.yaml"},
			expect: 1,
		},
		{
			desc: "helmrelease files match wildcard",
			files: []github.PullRequestFile{
				{Name: "src/package/helmrelease-with-suffix.yaml"},
				{Name: "src/package/prefix-helmrelease.yaml"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"lib", "src/*/*helmrelease*.yaml"},
			expect: 1,
		},
		{
			desc: "one file doesn't match wildcard",
			files: []github.PullRequestFile{
				{Name: "src/package/values.yaml"},
				{Name: "src/package/values2.yaml"},
			},
			paths:  []string{"src/*/values.yaml"},
			expect: env.DefaultApproverCount,
		},
		{
			desc: "all files matching multiple paths",
			files: []github.PullRequestFile{
				{Name: "lib/default.go"},
				{Name: "lib/db.go"},
			},
			paths:  []string{"lib/default", "lib/db"},
			expect: 1,
		},
		{
			desc:    "authors without match",
			author:  "foo",
			authors: []string{"bar"},
			files:   []github.PullRequestFile{{Name: "cannot_be_zero"}},
			expect:  env.DefaultApproverCount,
		},
		{
			desc:    "authors with match",
			author:  "foo",
			authors: []string{"bar", "foo"},
			files:   []github.PullRequestFile{{Name: "cannot_be_zero"}},
			expect:  1,
		},
	}
	for _, test := range cases {
		t.Run(test.desc, func(t *testing.T) {
			got := approverCount(test.authors, test.paths, test.author, test.files)
			require.Equal(t, test.expect, got)
		})
	}
}

type fakeGithub struct {
	files       []github.PullRequestFile
	pull        github.PullRequest
	jobs        []github.Job
	reviewers   []string
	reviews     []github.Review
	orgMembers  map[string]struct{}
	ref         github.Reference
	commitFiles []string
	comments    []github.Comment
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
	return f.jobs, nil
}

func (f *fakeGithub) DeleteWorkflowRun(ctx context.Context, organization string, repository string, runID int64) error {
	return nil
}

func (f *fakeGithub) IsOrgMember(ctx context.Context, user string, org string) (bool, error) {
	_, member := f.orgMembers[user]
	return member, nil
}

func (f *fakeGithub) CreateComment(ctx context.Context, organization string, repository string, number int, comment string) error {
	f.comments = append(f.comments, github.Comment{
		Body: comment,
	})

	return nil
}

func (f *fakeGithub) ListComments(ctx context.Context, organization string, repository string, number int) ([]github.Comment, error) {
	return f.comments, nil
}

func (f *fakeGithub) CreatePullRequest(ctx context.Context, organization string, repository string, title string, head string, base string, body string, draft bool) (int, error) {
	return 0, nil
}

func (f *fakeGithub) GetRef(ctx context.Context, organization string, repository string, ref string) (github.Reference, error) {
	return f.ref, nil
}

func (f *fakeGithub) ListCommitFiles(ctx context.Context, organization string, repository string, commitSHA string, pathPrefix string) ([]string, error) {
	return f.commitFiles, nil
}

func (f *fakeGithub) FetchAllOrgMembers(ctx context.Context, org string) ([]string, error) {
	orgMemberUserNameList := make([]string, 0)
	for member := range f.orgMembers {
		orgMemberUserNameList = append(orgMemberUserNameList, member)
	}
	return orgMemberUserNameList, nil
}

func TestSkipFileForSizeCheck(t *testing.T) {
	generatedFilePaths := []string{
		// go types from proto
		"api/types/types.pb.go",
		"api/gen/proto/go/teleport/accesslist/v1/accesslist.pb.go",
		// derived functions
		"api/types/accesslist/derived.gen.go",
		// generated docs
		"docs/pages/includes/helm-reference/zz_generated.teleport-kube-agent.mdx",
		"docs/pages/reference/infrastructure-as-code/operator-resources/resources-teleport-dev-accesslists.mdx",
		"docs/pages/reference/infrastructure-as-code/terraform-provider/resources/access_list.mdx",
		"docs/pages/reference/infrastructure-as-code/terraform-provider/data-sources/access_list.mdx",
		// CRDs
		"integrations/operator/config/crd/bases/resources.teleport.dev_accesslists.yaml",
		"examples/chart/teleport-cluster/charts/teleport-operator/operator-crds/resources.teleport.dev_accesslists.yaml",
		// CRs deepcopy
		"integrations/operator/apis/resources/v1/zz_generated.deepcopy.go",
		// TF schemas
		"integrations/terraform/tfschema/types_terraform.go",
		"integrations/terraform/tfschema/accesslist/v1/accesslist_terraform.go",
	}
	for _, file := range generatedFilePaths {
		assert.True(t, skipFileForSizeCheck(file), "file %q should be skipped for size check", file)
	}

	// This is not very scientific but here are a few files that are similar to
	// the generated ones but are hand-crafted. This test is not exhaustive, it
	// is only here to avoid an accidental catch-all regexp.
	nonGeneratedFilePaths := []string{
		"api/types/access_request.go",
		"api/types/accesslist/accesslist.go",
		"lib/accesslists/collection.go",
		"integrations/terraform/tfschema/accesslist/v1/custom_types.go",
		"integrations/operator/apis/resources/v1/accesslist_types.go",
		"docs/pages/identity-governance/access-lists/access-lists.mdx",
	}

	for _, file := range nonGeneratedFilePaths {
		assert.False(t, skipFileForSizeCheck(file), "file %q should not be skipped for size check", file)
	}
}
