/*
Copyright 2024 Gravitational, Inc.

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

package main

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/libs/git"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/trace"
)

// DefaultPRNumberCutoff is the date from which a commit subject's "(#N)"
// suffix is taken to be a pull request in the current repository.
//
// The repository was renamed and its pull request numbering restarted, so
// numbers below the current high-water mark appear twice in history: once for
// the old repository and once for this one. The last pull request numbered in
// the old scheme merged on 2026-08-14, and the restarted numbering did not
// reach those low numbers again until 2026-10-22, so commits from this date
// onwards can be numbered unambiguously.
const DefaultPRNumberCutoff = "2026-08-14"

var (
	// trailingPRRefs matches every trailing "(#N)" group on a subject.
	// Backports accumulate them, e.g. "Fix the thing (#100) (#200)" for a
	// backport of #100 merged as #200, so all of them have to come off to
	// leave a subject that compares equal across branches.
	trailingPRRefs = regexp.MustCompile(`(?:\s*\(#\d+\))+\s*$`)

	// leadingBullet matches the list marker GitHub puts on each squashed
	// commit when it builds a squash merge's message body.
	leadingBullet = regexp.MustCompile(`^\s*[*-]\s*`)

	// whitespaceRun matches a run of whitespace, collapsed so that rewrapped
	// subjects still compare equal.
	whitespaceRun = regexp.MustCompile(`\s+`)
)

// normalizeSubject reduces a commit subject to a key that compares equal
// across branches. The same change reaches a release branch with a different
// PR number than it had on the base branch, and arrives bulleted when it comes
// in through a squash merge, so neither can be part of the key.
func normalizeSubject(s string) string {
	s = leadingBullet.ReplaceAllString(s, "")
	s = trailingPRRefs.ReplaceAllString(s, "")
	s = whitespaceRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// releasedSet records what a base release ref already shipped, so that work
// already released is kept out of the next release's changelog.
//
// A plain "baseTag..headBranch" range is enough only while the base tag is an
// ancestor of the head branch. Release branches that chain off one another and
// take base branch work through squash merges and cherry-picks break that: the
// content is present but the commits are not, so git reports work as new that
// in fact shipped several releases ago.
type releasedSet struct {
	// prNumbers holds pull request numbers taken from commits reachable from
	// the base ref. Only numbers from this repository are trustworthy, so
	// these are restricted to commits at or after the numbering cutoff.
	prNumbers map[int]struct{}
	// subjects holds normalized subjects. Subjects are the only identity
	// available for commits that arrived through a squash merge, or that were
	// authored in a private fork and so carry a foreign PR number.
	subjects map[string]struct{}
}

// Suppression explains why a commit was left out of the changelog.
type Suppression struct {
	Commit git.Commit
	Reason string
}

// buildReleasedSet collects what baseRef already shipped, relative to headRef.
//
// Only baseRef's commits since the two refs diverged are considered. Scoping
// it that way keeps the whole shared history - and the stale PR numbers in it -
// out of the set, and makes the set empty when baseRef is an ancestor of
// headRef, where the plain range is already correct.
func buildReleasedSet(ctx context.Context, gh *github.Client, repo *git.Repo, ghOrg, ghRepo, baseRef, headRef string, cutoff time.Time) (*releasedSet, error) {
	mergeBase, err := repo.MergeBase(baseRef, headRef)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Not limited to first parents: a cherry-pick onto the release branch can
	// sit off the first-parent chain and still have shipped.
	chain, err := repo.CommitsInRange(mergeBase+".."+baseRef, false)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	set := &releasedSet{
		prNumbers: make(map[int]struct{}),
		subjects:  make(map[string]struct{}),
	}

	var squashed []int
	for _, commit := range chain {
		set.subjects[normalizeSubject(commit.Subject)] = struct{}{}

		// A squash merge's body lists the commits it squashed. It is already
		// local, so take it before asking GitHub for anything.
		for _, line := range strings.Split(commit.Body, "\n") {
			if key := normalizeSubject(line); key != "" {
				set.subjects[key] = struct{}{}
			}
		}

		if number := commit.PRNumber(); number != 0 {
			if !commit.Date.Before(cutoff) {
				set.prNumbers[number] = struct{}{}
			}
			squashed = append(squashed, number)
		}
	}

	// Ask GitHub for the commits behind each PR merged into the chain. This is
	// the only way back to the commits in a squash merge whose message body was
	// trimmed to the PR title.
	messages, err := gh.PullRequestCommitMessages(ctx, ghOrg, ghRepo, squashed)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, prMessages := range messages {
		for _, message := range prMessages {
			// Every line, not just the subject: squashed commits bundle several
			// changes, listing all but the first in the message body.
			for _, line := range strings.Split(message, "\n") {
				// Subjects only, never the PR numbers in them. A squash merge
				// can carry commits authored in a private fork, whose "(#N)"
				// suffixes number that fork's pull requests, not this
				// repository's. Trusting those numbers suppresses whichever of
				// this repository's pull requests happens to share a number.
				if key := normalizeSubject(line); key != "" {
					set.subjects[key] = struct{}{}
				}
			}
		}
	}

	return set, nil
}

// suppressed partitions commits into those to put in the changelog and those
// baseRef already shipped.
func (s *releasedSet) suppressed(commits []git.Commit) (keep []git.Commit, dropped []Suppression) {
	for _, commit := range commits {
		if number := commit.PRNumber(); number != 0 {
			if _, ok := s.prNumbers[number]; ok {
				dropped = append(dropped, Suppression{Commit: commit, Reason: "pull request already released"})
				continue
			}
		}
		if _, ok := s.subjects[normalizeSubject(commit.Subject)]; ok {
			dropped = append(dropped, Suppression{Commit: commit, Reason: "subject already released"})
			continue
		}
		keep = append(keep, commit)
	}
	return keep, dropped
}

// prNumbersOf returns the unique pull request numbers of the given commits, in
// order. Commits without a PR reference are skipped.
func prNumbersOf(commits []git.Commit) []int {
	var numbers []int
	seen := make(map[int]struct{}, len(commits))
	for _, commit := range commits {
		number := commit.PRNumber()
		if number == 0 {
			continue
		}
		if _, ok := seen[number]; ok {
			continue
		}
		seen[number] = struct{}{}
		numbers = append(numbers, number)
	}
	return numbers
}
