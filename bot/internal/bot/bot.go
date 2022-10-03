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
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"

	"github.com/gravitational/trace"
)

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
	ListComments(ctx context.Context, organization string, repository string, number int) ([]string, error)

	// ListWorkflows lists all workflows within a repository.
	ListWorkflows(ctx context.Context, organization string, repository string) ([]github.Workflow, error)

	// ListWorkflowRuns is used to list all workflow runs for an ID.
	ListWorkflowRuns(ctx context.Context, organization string, repository string, branch string, workflowID int64) ([]github.Run, error)

	// ListWorkflowJobs lists all jobs for a workflow run.
	ListWorkflowJobs(ctx context.Context, organization string, repository string, runID int64) ([]github.Job, error)

	// DeleteWorkflowRun is used to delete a workflow run.
	DeleteWorkflowRun(ctx context.Context, organization string, repository string, runID int64) error
}

// Config contains configuration for the bot.
type Config struct {
	// GitHub is a GitHub client.
	GitHub Client

	// Environment holds information about the workflow run event.
	Environment *env.Environment

	// Review is used to get code and docs reviewers.
	Review *review.Assignments
}

// CheckAndSetDefaults checks and sets defaults.
func (c *Config) CheckAndSetDefaults() error {
	if c.GitHub == nil {
		return trace.BadParameter("missing parameter GitHub")
	}
	if c.Environment == nil {
		return trace.BadParameter("missing parameter Environment")
	}
	if c.Review == nil {
		return trace.BadParameter("missing parameter Review")
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
func classifyChanges(e *env.Environment, files []github.PullRequestFile) (docs bool, code bool, err error) {
	switch e.Repository {
	case env.TeleportRepo:
		for _, file := range files {
			if strings.HasPrefix(file.Name, "docs/") {
				docs = true
			} else {
				code = true
			}
		}
	default:
		code = true
	}
	return docs, code, nil
}

func isLargePR(files []github.PullRequestFile) bool {
	var additions, deletions int
	for _, f := range files {
		if isAutoGeneratedFile(f.Name) {
			continue
		}
		additions += f.Additions
		deletions += f.Deletions
	}
	return additions-deletions > 1500
}

func isAutoGeneratedFile(name string) bool {
	return strings.HasSuffix(name, ".golden") ||
		strings.HasSuffix(name, ".pb.go") ||
		strings.HasSuffix(name, "_pb.js") ||
		strings.HasSuffix(name, "_pb.d.ts") ||
		strings.Contains(name, "webassets/") ||
		strings.Contains(name, "vendor/")
}

func isReleaseBranch(branch string) bool {
	return strings.HasPrefix(branch, "branch/")
}

func isBotBackportBranch(branch string) bool {
	return strings.HasPrefix(branch, "bot/backport")
}
