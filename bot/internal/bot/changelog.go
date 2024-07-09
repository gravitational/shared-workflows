/*
Copyright 2023 Gravitational, Inc.

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
	"log"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gravitational/trace"
)

const NoChangelogLabel string = "no-changelog"
const ChangelogPrefix string = "changelog: "
const ChangelogRegex string = "(?mi)^changelog: .*"

// Checks if the PR contains a changelog entry in the PR body, or a "no-changelog" label.
//
// A few tests are performed on the extracted changelog entry to ensure it conforms to a
// common standard.
func (b *Bot) CheckChangelog(ctx context.Context) error {
	pull, err := b.c.GitHub.GetPullRequest(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err, "failed to retrieve pull request for https://github.com/%s/%s/pull/%d", b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)
	}

	if slices.Contains(pull.UnsafeLabels, NoChangelogLabel) {
		log.Printf("PR contains %q label, skipping changelog check", NoChangelogLabel)
		return nil
	}

	files, err := b.c.GitHub.ListFiles(
		ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err, "failed to retrieve pull request files for https://github.com/%s/%s/pull/%d", b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)

	}

	c := classifyChanges(b.c, files)
	if c.Docs && !c.Code {
		log.Print("PR contains only docs changes. No need for a changelog entry.")
		return nil
	}

	changelogEntries := b.getChangelogEntries(pull.UnsafeBody)
	if len(changelogEntries) == 0 {
		return trace.BadParameter("Changelog entry not found in the PR body. Please add a %q label to the PR, or changelog lines starting with `%s` followed by the changelog entries for the PR.", NoChangelogLabel, ChangelogPrefix)
	}

	for _, changelogEntry := range changelogEntries {
		err = b.validateChangelogEntry(ctx, changelogEntry)
		if err != nil {
			return trace.Wrap(err, "failed to validate changelog entry %q", changelogEntry)
		}
	}

	return nil
}

func (b *Bot) getChangelogEntries(prBody string) []string {
	changelogRegex := regexp.MustCompile(ChangelogRegex)

	changelogMatches := changelogRegex.FindAllString(prBody, -1)
	for i, changelogMatch := range changelogMatches {
		changelogMatches[i] = changelogMatch[len(ChangelogPrefix):] // Case insensitive prefix removal
	}

	if len(changelogMatches) > 0 {
		log.Printf("Found changelog entries %v", changelogMatches)
	}
	return changelogMatches
}

// Checks for common issues with the changelog entry.
// This is not intended to be comprehensive, rather, it is intended to cover the majority of problems.
func (b *Bot) validateChangelogEntry(ctx context.Context, changelogEntry string) error {
	changelogEntry = strings.ToLower(strings.TrimSpace(changelogEntry)) // Format the entry for easy validation
	if changelogEntry == "" {
		return trace.BadParameter("The changelog entry must contain one or more non-whitespace characters.")
	}

	if !unicode.IsLetter([]rune(changelogEntry)[0]) {
		return trace.BadParameter("The changelog entry must start with a letter.")
	}

	if strings.HasPrefix(changelogEntry, "backport of") ||
		strings.HasPrefix(changelogEntry, "backports") {
		return trace.BadParameter("The changelog entry must contain the actual change, not a reference to the source PR of the backport.")
	}

	if strings.Contains(changelogEntry, "](") {
		return trace.BadParameter("The changelog entry must not contain a Markdown link or image.")
	}

	if strings.Contains(changelogEntry, "```") {
		return trace.BadParameter("The changelog entry must not contain a multiline code block.")
	}

	if changelogEntry == "none" {
		return trace.BadParameter("The %q label must be set instead of listing 'none' as the changelog entry.", NoChangelogLabel)
	}

	return nil
}
