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
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/gravitational/shared-workflows/bot/internal/github"

	"github.com/gravitational/trace"
)

// Backport will create backport Pull Requests (if requested) when a Pull
// Request is merged.
func (b *Bot) Backport(ctx context.Context) error {
	internal, err := b.isInternal(ctx)
	if err != nil {
		return trace.Wrap(err, "checking for internal author")
	}
	if !internal {
		return trace.BadParameter("automatic backports are only supported for internal contributors")
	}

	pull, err := b.c.GitHub.GetPullRequestWithCommits(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Printf("backporting %v/%v#%v",
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)

	// If this workflow is running on a release branch, then it means
	// a backport was merged and there's no need to open additional
	// backport PRs. We can safely delete the remote branch though,
	// because we know this is an internal contributor's merged PR.
	if isReleaseBranch(b.c.Environment.UnsafeBase) &&
		isBotBackportBranch(b.c.Environment.UnsafeHead) &&
		!pull.Fork {
		log.Printf("backport merged to %v, deleting branch %v", b.c.Environment.UnsafeBase, b.c.Environment.UnsafeHead)
		return trace.Wrap(git("push", "origin", "--delete", b.c.Environment.UnsafeHead))
	}

	// Extract backport branches names from labels attached to the Pull
	// Request. If no backports were requested, return right away.
	branches := findBranches(pull.UnsafeLabels)
	if len(branches) == 0 {
		return nil
	}

	log.Printf("target branches: %v", strings.Join(branches, ", "))

	// Get workflow logs URL, will be attached to any backport failure.
	u, err := b.workflowLogsURL(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.RunID)
	if err != nil {
		return trace.Wrap(err)
	}

	var rows []row

	// Loop over all requested backport branches and create backport branch and
	// GitHub Pull Request.
	for _, base := range branches {
		head := b.backportBranchName(base)

		// Create and push git branch for backport to GitHub.
		err := b.createBackportBranch(ctx,
			b.c.Environment.Organization,
			b.c.Environment.Repository,
			b.c.Environment.Number,
			base,
			pull,
			head,
			git,
		)
		if err != nil {
			log.Printf("Failed to create backport branch:\n%v\n", trace.DebugReport(err))
			rows = append(rows, row{
				Branch: base,
				Failed: true,
				Link:   u,
			})
			continue
		}

		rows = append(rows, row{
			Branch: base,
			Failed: false,
			Link: url.URL{
				Scheme: "https",
				Host:   "github.com",
				// Both base and head are safe to put into the URL: base has
				// had the "branchPattern" regexp run against it and head is
				// formed from base so an attacker can not control the path.
				Path: path.Join(b.c.Environment.Organization, b.c.Environment.Repository, "compare", fmt.Sprintf("%v...%v", base, head)),
				RawQuery: url.Values{
					"expand": []string{"1"},
					"title":  []string{fmt.Sprintf("[%v] %v", strings.Trim(base, "branch/"), pull.UnsafeTitle)},
					"body":   []string{fmt.Sprintf("Backport #%v to %v", b.c.Environment.Number, base)},
				}.Encode(),
			},
		})
	}

	// Leave a comment on the Pull Request with a table that outlines the
	// requested backports and outcome.
	err = b.updatePullRequest(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		data{
			Author: b.c.Environment.Author,
			Rows:   rows,
		})
	return trace.Wrap(err)
}

// BackportLocal executes dry run backport workflow locally. No git commands
// are actually executed, just printed in the console.
func (b *Bot) BackportLocal(ctx context.Context, branch string) error {
	pull, err := b.c.GitHub.GetPullRequestWithCommits(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	err = b.createBackportBranch(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		branch,
		pull,
		b.backportBranchName(branch),
		gitDryRun)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

func (b *Bot) backportBranchName(base string) string {
	return fmt.Sprintf("bot/backport-%v-%v", b.c.Environment.Number, base)
}

// findBranches looks through the labels attached to a Pull Request for all the
// backport branches the user requested.
func findBranches(labels []string) []string {
	var branches []string

	for _, label := range labels {
		if !strings.HasPrefix(label, "backport/") {
			continue
		}

		branch := strings.TrimPrefix(label, "backport/")
		if !branchPattern.MatchString(branch) {
			continue
		}

		branches = append(branches, branch)
	}

	sort.Strings(branches)
	return branches
}

// createBackportBranch will create and push a git branch with all the commits
// from a Pull Request on it.
//
// TODO(russjones): Refactor to use go-git (so similar git library) instead of
// executing git from disk.
func (b *Bot) createBackportBranch(ctx context.Context, organization string, repository string, number int, base string, pull github.PullRequest, newHead string, git func(...string) error) error {
	log.Println("--> Backporting to", base, "<--")

	if err := git("config", "--global", "user.name", "github-actions"); err != nil {
		log.Printf("Failed to set user.name: %v.", err)
	}
	if err := git("config", "--global", "user.email", "github-actions@goteleport.com"); err != nil {
		log.Printf("Failed to set user.email: %v.", err)
	}

	// Download base and head from origin (GitHub).
	if err := git("fetch", "origin", base, pull.UnsafeHead.Ref); err != nil {
		return trace.Wrap(err)
	}

	// Checkout the base branch.
	if err := git("checkout", base); err != nil {
		return trace.Wrap(err)
	}

	// Checkout the new backport branch.
	if err := git("checkout", "-b", newHead); err != nil {
		return trace.Wrap(err)
	}

	// Cherry-pick all commits from the PR to the backport branch.
	for _, commit := range pull.Commits {
		if err := git("cherry-pick", commit); err != nil {
			// If cherry-pick fails with conflict, abort it, otherwise we
			// won't be able to switch branch for the next backport.
			if errAbrt := git("cherry-pick", "--abort"); errAbrt != nil {
				return trace.NewAggregate(err, errAbrt)
			}
			return trace.Wrap(err)
		}
	}

	// Push the backport branch to Github.
	if err := git("push", "origin", newHead); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// updatePullRequest will leave a comment on the Pull Request with the status
// of backports.
func (b *Bot) updatePullRequest(ctx context.Context, organization string, repository string, number int, d data) error {
	var buf bytes.Buffer

	t := template.Must(template.New("table").Parse(table))
	if err := t.Execute(&buf, d); err != nil {
		return trace.Wrap(err)
	}

	err := b.c.GitHub.CreateComment(ctx,
		organization,
		repository,
		number,
		buf.String())
	return trace.Wrap(err)
}

// workflowLogsURL returns the workflow logs URL.
func (b *Bot) workflowLogsURL(ctx context.Context, organization string, repository string, runID int64) (url.URL, error) {
	jobs, err := b.c.GitHub.ListWorkflowJobs(ctx,
		organization,
		repository,
		runID)
	if err != nil {
		return url.URL{}, trace.Wrap(err)
	}
	if len(jobs) != 1 {
		return url.URL{}, trace.BadParameter("invalid number of jobs %v", len(jobs))
	}

	return url.URL{
		Scheme:   "https",
		Host:     "github.com",
		Path:     path.Join(b.c.Environment.Organization, b.c.Environment.Repository, "runs", strconv.FormatInt(jobs[0].ID, 10)),
		RawQuery: url.Values{"check_suite_focus": []string{"true"}}.Encode(),
	}, nil
}

// git will execute the "git" program on disk.
func git(args ...string) error {
	log.Println("Running:", "git", strings.Join(args, " "))
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return trace.BadParameter(string(bytes.TrimSpace(out)))
	}
	return nil
}

// gitDryRun logs "git" commands in the console.
func gitDryRun(args ...string) error {
	log.Println("Running: git", strings.Join(args, " "))
	return nil
}

// data is injected into the template to render outcome of all backport
// attempts.
type data struct {
	// Author of the Pull Request. Used to @author on GitHub so they get a
	// notification.
	Author string

	// Rows represent backports.
	Rows []row
}

// row represents a single backport attempt.
type row struct {
	// Failed is used to indicate if this backport failed.
	Failed bool

	// Branch is the name of the backport branch.
	Branch string

	// Link is a URL pointing to the created backport Pull Request.
	Link url.URL
}

// table is a template that is written to the origin GitHub Pull Request with
// the outcome of the backports.
const table = `
@{{.Author}} See the table below for backport results.

| Branch | Result |
|--------|--------|
{{- range .Rows}}
| {{.Branch}} | {{if .Failed}}[Failed]({{.Link}}){{else}}[Create PR]({{.Link}}){{end}} |
{{- end}}
`

// branchPattern defines valid backport branch names.
var branchPattern = regexp.MustCompile(`(^branch\/v[0-9]+$)|(^master$)`)
