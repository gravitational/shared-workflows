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
	"slices"
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

	g := gitFunc(git)
	if b.c.Git != nil {
		g = b.c.Git
	}

	// The commits being backported are the same for every target branch, so fetch them once here.
	if err := prepareRepo(ctx, b.c.Environment.Number, len(pull.Commits), g); err != nil {
		return trace.Wrap(err, "preparing repository")
	}

	// Loop over all requested backport branches and create backport branch and
	// GitHub Pull Request.
	for _, base := range branches {
		head := b.backportBranchName(base)

		// Create and push git branch for backport to GitHub.
		err := b.createBackportBranch(ctx, base, pull, head, g)
		if err != nil {
			log.Printf("Failed to create backport branch:\n%v\n", trace.DebugReport(err))
			rows = append(rows, row{
				Branch: base,
				Failed: true,
				Link:   u,
			})
			continue
		}

		// Exempt the backport from the changelog and test plan checks when
		// the original PR has nothing to carry over for them.
		var labels []string
		bodyText := fmt.Sprintf("Backport #%v to %v", b.c.Environment.Number, base)
		if entries := b.getChangelogEntries(pull.UnsafeBody); len(entries) > 0 && !slices.Contains(pull.UnsafeLabels, NoChangelogLabel) {
			bodyText += "\n\n"
			for _, entry := range entries {
				bodyText += fmt.Sprintf("%s%s\n", ChangelogPrefix, entry)
			}
		} else {
			labels = append(labels, NoChangelogLabel)
		}
		testPlan := getManualTestPlan(pull.UnsafeBody)
		if testPlan == "" {
			labels = append(labels, noTestPlanLabel)
		}

		link := url.URL{
			Scheme: "https",
			Host:   "github.com",
			// Both base and head are safe to put into the URL: base has
			// had the "branchPattern" regexp run against it and head is
			// formed from base so an attacker can not control the path.
			Path: path.Join(b.c.Environment.Organization, b.c.Environment.Repository, "compare", fmt.Sprintf("%v...%v", base, head)),
		}
		query := url.Values{
			"expand": []string{"1"},
			"title":  []string{fmt.Sprintf("[%v] %v", strings.Trim(base, "branch/"), pull.UnsafeTitle)},
			"body":   []string{bodyText},
			"labels": labels,
		}
		link.RawQuery = query.Encode()

		// Carry the manual test plan over to the backport, unless that makes
		// the link longer than GitHub accepts. The author then adds it by hand.
		if testPlan != "" {
			withPlan := link
			query.Set("body", strings.TrimRight(bodyText, "\n")+"\n\n"+testPlan)
			withPlan.RawQuery = query.Encode()
			if len(withPlan.String()) <= maxLinkLength {
				link = withPlan
			} else {
				log.Printf("Not including the manual test plan for %v: link would be %d bytes", base, len(withPlan.String()))
			}
		}

		rows = append(rows, row{
			Branch: base,
			Failed: false,
			Link:   link,
		})
	}

	// Leave a comment on the Pull Request with a table that outlines the
	// requested backports and outcome.
	//
	// Detach from ctx: if we got this far, the branches were already pushed and
	// all we need to do is post the comment with PR links.
	commentCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), github.ClientTimeout)
	defer cancel()

	err = b.updatePullRequest(commentCtx,
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

	if err := prepareRepo(ctx, b.c.Environment.Number, len(pull.Commits), gitDryRun); err != nil {
		return trace.Wrap(err, "preparing repository")
	}

	err = b.createBackportBranch(ctx,
		branch,
		pull,
		b.backportBranchName(branch),
		gitDryRun)
	if err != nil {
		return trace.Wrap(err)
	}

	return nil
}

const botBackportBranchPrefix = "bot/backport"

// maxLinkLength caps the "Create PR" link. GitHub rejects URLs over 8 KiB
// outright and its compare page started failing at around 6.5 KiB in testing,
// so stay well below both.
const maxLinkLength = 4096

func (b *Bot) backportBranchName(base string) string {
	return fmt.Sprintf("%s-%v-%v", botBackportBranchPrefix, b.c.Environment.Number, base)
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

// prRef is the local ref the commits from a Pull Request are fetched into.
func prRef(number int) string {
	return fmt.Sprintf("refs/remotes/origin/pr/%d", number)
}

// baseRef is the local ref the tip of a backport's base branch is fetched into.
func baseRef(base string) string {
	return "refs/remotes/origin/" + base
}

// prepareRepo configures git and fetches the commits that are about to be backported.
func prepareRepo(ctx context.Context, number int, commits int, git gitFunc) error {
	if err := git(ctx, "config", "--global", "user.name", "github-actions"); err != nil {
		log.Printf("Failed to set user.name: %v.", err)
	}
	if err := git(ctx, "config", "--global", "user.email", "github-actions@goteleport.com"); err != nil {
		log.Printf("Failed to set user.email: %v.", err)
	}

	// The teleport repo is large, so we don't want to do any more work than necessary here.
	// - there are a lot of tags, and we don't need them
	// - we don't need the entire history, only the commits being cherry-picked and their parent
	return trace.Wrap(git(ctx,
		"fetch", "--no-tags", "--depth", strconv.Itoa(commits+1), "origin",
		fmt.Sprintf("+refs/pull/%d/head:%s", number, prRef(number))))
}

// createBackportBranch will create and push a git branch with all the commits
// from a Pull Request on it. [prepareRepo] must have run first.
//
// TODO(russjones): Refactor to use go-git (so similar git library) instead of
// executing git from disk.
func (b *Bot) createBackportBranch(ctx context.Context, base string, pull github.PullRequest, newHead string, git gitFunc) error {
	log.Println("--> Backporting to", base, "<--")

	// Only the tip of the base branch is needed.
	if err := git(ctx, "fetch", "--no-tags", "--depth", "1", "origin",
		fmt.Sprintf("+refs/heads/%s:%s", base, baseRef(base))); err != nil {
		return trace.Wrap(err)
	}

	// Point the backport branch at the tip of the base branch.
	if err := git(ctx, "checkout", "-B", newHead, "--no-track", baseRef(base)); err != nil {
		return trace.Wrap(err)
	}

	// Cherry-pick all commits from the PR to the backport branch.
	for _, commit := range pull.Commits {
		if err := git(ctx, "cherry-pick", commit); err != nil {
			// If cherry-pick fails with conflict, abort it, otherwise we
			// won't be able to switch branch for the next backport.
			if errAbrt := git(ctx, "cherry-pick", "--abort"); errAbrt != nil {
				return trace.NewAggregate(err, errAbrt)
			}
			return trace.Wrap(err)
		}
	}

	// Push the backport branch to Github.
	if err := git(ctx, "push", "origin", newHead); err != nil {
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

type gitFunc func(ctx context.Context, args ...string) error

// git will execute the "git" program on disk.
func git(ctx context.Context, args ...string) error {
	log.Println("Running:", "git", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return trace.BadParameter("git failed: %s", string(bytes.TrimSpace(out)))
	}
	return nil
}

// gitDryRun logs "git" commands in the console.
func gitDryRun(_ context.Context, args ...string) error {
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
