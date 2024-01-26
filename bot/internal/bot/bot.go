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
	"regexp"
	"strings"

	"github.com/gravitational/trace"
	"golang.org/x/exp/slices"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
)

// isCRDRegex matches Teleport operator CRD file paths.
// Those files receive a special treatment as they're automatically generated.
var isCRDRegex = regexp.MustCompile(`.*/resources\.teleport\.dev_[[:alpha:]]+\.yaml$`)

// Client implements the GitHub API.
type Client interface {
	// RequestReviewers is used to assign reviewers to a Pull Request.
	RequestReviewers(ctx context.Context, organization string, repository string, number int, reviewers []string) error

	// DismissReviewers is used to remove the review request from a Pull Request.
	DismissReviewers(ctx context.Context, organization string, repository string, number int, reviewers []string) error

	// ListReviews is used to list all submitted reviews for a PR.
	ListReviews(ctx context.Context, organization string, repository string, number int) ([]github.Review, error)

	// ListReviewers returns a list of reviewers that have yet to submit a review.
	ListReviewers(ctx context.Context, organization string, repository string, number int) ([]string, error)

	// GetPullRequest returns a specific Pull Request.
	GetPullRequest(ctx context.Context, organization string, repository string, number int) (github.PullRequest, error)

	// GetPullRequestWithCommits returns a specific Pull Request with commits.
	GetPullRequestWithCommits(ctx context.Context, organization string, repository string, number int) (github.PullRequest, error)

	// CreatePullRequest will create a Pull Request.
	CreatePullRequest(ctx context.Context, organization string, repository string, title string, head string, base string, body string, draft bool) (int, error)

	// ListPullRequests returns a list of Pull Requests.
	ListPullRequests(ctx context.Context, organization string, repository string, state string) ([]github.PullRequest, error)

	// ListFiles is used to list all the files within a Pull Request.
	ListFiles(ctx context.Context, organization string, repository string, number int) ([]github.PullRequestFile, error)

	// AddLabels will add labels to an Issue or Pull Request.
	AddLabels(ctx context.Context, organization string, repository string, number int, labels []string) error

	// CreateComment will leave a comment on an Issue or Pull Request.
	CreateComment(ctx context.Context, organization string, repository string, number int, comment string) error

	// ListComments will list all comments on an Issue or Pull Request.
	ListComments(ctx context.Context, organization string, repository string, number int) ([]github.Comment, error)

	// ListWorkflows lists all workflows within a repository.
	ListWorkflows(ctx context.Context, organization string, repository string) ([]github.Workflow, error)

	// ListWorkflowRuns is used to list all workflow runs for an ID.
	ListWorkflowRuns(ctx context.Context, organization string, repository string, branch string, workflowID int64) ([]github.Run, error)

	// ListWorkflowJobs lists all jobs for a workflow run.
	ListWorkflowJobs(ctx context.Context, organization string, repository string, runID int64) ([]github.Job, error)

	// DeleteWorkflowRun is used to delete a workflow run.
	DeleteWorkflowRun(ctx context.Context, organization string, repository string, runID int64) error

	// IsOrgMember checks whether [user] is a member of GitHub orgainzation [org].
	IsOrgMember(ctx context.Context, user string, org string) (bool, error)

	// GetRef returns a Reference representing the provided ref name.
	GetRef(ctx context.Context, organization string, repository string, ref string) (github.Reference, error)

	// ListCommitFiles returns all filenames recursively from the tree at a given commit SHA whose prefix matches pathPrefix.
	ListCommitFiles(ctx context.Context, organization string, repository string, sha string, path string) ([]string, error)
}

// Config contains configuration for the bot.
type Config struct {
	// GitHub is a GitHub client.
	GitHub Client

	// Environment holds information about the workflow run event.
	Environment *env.Environment

	// Review is used to get code and docs reviewers.
	Review *review.Assignments

	// Git is used to run git commands, uses dry run in tests.
	Git func(...string) error
}

// CheckAndSetDefaults checks and sets defaults.
func (c *Config) CheckAndSetDefaults() error {
	if c.GitHub == nil {
		return trace.BadParameter("missing parameter GitHub")
	}
	if c.Environment == nil {
		return trace.BadParameter("missing parameter Environment")
	}

	return nil
}

// Bot performs repository management.
type Bot struct {
	c *Config
}

// New returns a new repository management bot.
func New(c *Config) (*Bot, error) {
	if err := c.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	return &Bot{
		c: c,
	}, nil
}

// classifyChanges determines whether the PR contains code changes
// and/or docs changes.
func classifyChanges(c *Config, files []github.PullRequestFile) env.Changes {
	ch := env.Changes{
		Large:   !c.Environment.IsCloudDeployBranch() && xlargeRequiresAdminApproval(files),
		Release: isReleasePR(c.Environment, files),
		ApproverCount: approverCount(
			review.SingleApproverAuthors(c.Environment.Repository),
			review.SingleApproverPaths(c.Environment.Repository),
			c.Environment.Author,
			files,
		),
	}
	switch c.Environment.Repository {
	case env.TeleportRepo:
		for _, file := range files {
			if strings.HasPrefix(file.Name, "docs/") ||
				file.Name == "CHANGELOG.md" {
				ch.Docs = true
			} else {
				ch.Code = true
			}
		}
	default:
		ch.Code = true
	}
	return ch
}

// approverCount returns the number of required approvers for the PR by comparing the files included
// in the PR against a set of paths that only require a single approver and the PR author against
// a set of authors that only require a single approver. 1 is returned when all of the
// files match a single approver path or the PR author matches one of the authors, otherwise
// env.DefaultApproverCount is returned.
func approverCount(authors, paths []string, author string, files []github.PullRequestFile) int {
	if len(files) == 0 {
		return env.DefaultApproverCount
	}

	// check if pr author only requires a single approval
	if slices.Contains(authors, author) {
		return 1
	}

	// check if pr files only require a single approval
	if len(paths) == 0 {
		return env.DefaultApproverCount
	}
	for _, file := range files {
		if !slices.ContainsFunc(paths, func(path string) bool {
			return strings.HasPrefix(file.Name, path)
		}) {
			return env.DefaultApproverCount
		}
	}
	return 1
}

// isReleasePR applies a number of heuristics to the PR changeset to determine
// whether it's a release PR.
func isReleasePR(e *env.Environment, files github.PullRequestFiles) bool {
	// Check if the branch name starts with release/ prefix.
	if !strings.HasPrefix(e.UnsafeHead, "release/") {
		return false
	}
	// Check that the files that typically change in a release PR are there.
	for _, releasePRFile := range releasePRFiles {
		if !files.HasFile(releasePRFile.Name) {
			return false
		}
	}
	// Check that the PR doesn't contain any other code changes.
	for _, sourceFile := range files.SourceFiles() {
		if !releasePRFiles.HasFile(sourceFile.Name) {
			return false
		}
	}
	return true
}

// releasePRFiles is a list of files that change when making a new release.
//
// This is not an exhaustive list but should be a good enough indicator that
// the changeset is a release PR.
var releasePRFiles = github.PullRequestFiles{
	{Name: "CHANGELOG.md"},
	{Name: "Makefile"},
	{Name: "version.go"},
	{Name: "api/version.go"},
	{Name: "integrations/kube-agent-updater/version.go"},
}

func xlargeRequiresAdminApproval(files []github.PullRequestFile) bool {
	return prSize(files) == xlarge
}

type sizeLabel string

const (
	small  sizeLabel = "size/sm"
	medium sizeLabel = "size/md"
	large  sizeLabel = "size/lg"
	xlarge sizeLabel = "size/xl"
)

func prSize(files []github.PullRequestFile) sizeLabel {
	var additions, deletions int
	for _, f := range files {
		if skipFileForSizeCheck(f.Name) {
			continue
		}
		additions += f.Additions
		deletions += f.Deletions
	}
	delta := additions - deletions
	switch {
	case delta < 100:
		return small
	case delta < 600:
		return medium
	case delta < 1500:
		return large
	default:
		return xlarge
	}
}

func skipFileForSizeCheck(name string) bool {
	return strings.HasSuffix(name, ".golden") ||
		strings.HasSuffix(name, ".pb.go") ||
		strings.HasSuffix(name, "_pb.js") ||
		strings.HasSuffix(name, "_pb.d.ts") ||
		strings.HasSuffix(name, "_pb.ts") ||
		strings.HasSuffix(name, "_pb.grpc-client.ts") ||
		strings.HasSuffix(name, "_pb.grpc-server.ts") ||
		strings.HasSuffix(name, ".json") ||
		strings.Contains(name, "webassets/") ||
		strings.Contains(name, "vendor/") ||
		isCRDRegex.MatchString(name)
}

func isReleaseBranch(branch string) bool {
	return strings.HasPrefix(branch, "branch/")
}

func (b *Bot) isInternal(ctx context.Context) (bool, error) {
	// check fast-path first - if the author is explicitly listed as
	// an internal reviewer then we don't need to make a network call
	if b.c.Review.IsInternal(b.c.Environment.Author) {
		return true, nil
	}

	return b.c.GitHub.IsOrgMember(ctx, b.c.Environment.Author, "gravitational")
}
