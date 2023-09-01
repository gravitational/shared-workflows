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
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"

	"github.com/gravitational/trace"
	"k8s.io/utils/strings/slices"
)

const NoChangelogLabel string = "no-changelog"
const ChangelogPrefix string = "changelog: "
const ChangelogTag string = "changelog"

func (b *Bot) Changelog(ctx context.Context) error {
	pull, err := b.c.GitHub.GetPullRequest(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err, "failed to retrieve pull request for https://github.com/%s/%s/pull/%d", b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)
	}

	if slices.Contains(pull.UnsafeLabels, NoChangelogLabel) {
		log.Printf("PR contains no changelog label %q, skipping changelog check", NoChangelogLabel)
		return nil
	}

	changelogEntry, err := b.getChangelogEntry(ctx, pull.UnsafeBody)
	if err != nil {
		return trace.Wrap(err, "failed to get changelog entry")
	}

	err = b.validateChangelogEntry(ctx, changelogEntry)
	if err != nil {
		return trace.Wrap(err, "failed to validate changelog entry")
	}

	err = b.deleteComments(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to delete changelog comments")
	}

	return nil
}

func (b *Bot) getChangelogEntry(ctx context.Context, prBody string) (string, error) {
	changelogRegexString := fmt.Sprintf("(?mi)^%s.*", regexp.QuoteMeta(ChangelogPrefix))
	changelogRegex, err := regexp.Compile(changelogRegexString)
	if err != nil {
		return "", trace.Wrap(err, "failed to compile changelog prefix regex %q", changelogRegexString)
	}

	changelogMatches := changelogRegex.FindAllString(prBody, 2) // Two or more changelog entries is invalid, so no need to search for more than two.
	if len(changelogMatches) == 0 {
		return "", b.logFailedCheck(ctx, "Changelog entry not found in the PR body. Please add a %q label to the PR, or a changelog line starting with `%s` followed by the changelog entry for the PR.", NoChangelogLabel, ChangelogPrefix)
	}

	if len(changelogMatches) > 1 {
		return "", b.logFailedCheck(ctx, "Found two or more changelog entries in the PR body: %v", changelogMatches)
	}

	changelogEntry := changelogMatches[0][len(ChangelogPrefix):] // Case insensitive prefix removal
	log.Printf("Found changelog entry %q matching %q", changelogEntry, changelogRegexString)
	return changelogEntry, nil
}

// Checks for common issues with the changelog entry.
// This is not intended to be comprehensive, rather, it is intended to cover the majority of problems.
func (b *Bot) validateChangelogEntry(ctx context.Context, changelogEntry string) error {
	if strings.TrimSpace(changelogEntry) == "" {
		return b.logFailedCheck(ctx, "The changelog entry must contain one or more non-whitespace characters")
	}

	if !unicode.IsLetter([]rune(changelogEntry)[0]) {
		return b.logFailedCheck(ctx, "The changelog entry must start with a letter")
	}

	if unicode.IsLower([]rune(changelogEntry)[0]) {
		return b.logFailedCheck(ctx, "The changelog entry must start with an uppercase character")
	}

	if strings.TrimSpace(changelogEntry) != changelogEntry {
		return b.logFailedCheck(ctx, "The changelog entry must not contain leading or trailing whitespace")
	}

	if strings.HasPrefix(strings.ToLower(changelogEntry), "backport of") ||
		strings.HasPrefix(strings.ToLower(changelogEntry), "backports") {
		return b.logFailedCheck(ctx, "The changelog entry must contain the actual change, not a reference to the source PR of the backport.")
	}

	if unicode.IsPunct([]rune(changelogEntry)[len(changelogEntry)-1:][0]) {
		return b.logFailedCheck(ctx, "The changelog entry must not end with punctuation.")
	}

	if strings.Contains(changelogEntry, "](") {
		return b.logFailedCheck(ctx, "The changelog entry must not contain a Markdown link or image")
	}

	if strings.Contains(changelogEntry, "```") {
		return b.logFailedCheck(ctx, "The changelog entry must not contain a multiline code block")
	}

	return nil
}

func (b *Bot) logFailedCheck(ctx context.Context, format string, args ...interface{}) error {
	createOrUpdateCount, err := b.c.GitHub.CreateOrUpdateStatefulComment(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		fmt.Sprintf(format, args...),
		ChangelogTag,
	)
	if err != nil {
		return trace.Wrap(err, "failed to create or update the changelog comment")
	}
	if createOrUpdateCount < 1 {
		return trace.Errorf("failed to create or update the changelog comment")
	}

	return trace.Errorf(format, args...)
}

func (b *Bot) deleteComments(ctx context.Context) error {
	_, err := b.c.GitHub.DeleteStatefulComment(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		ChangelogTag,
	)

	if err != nil {
		return trace.Wrap(err, "failed to delete changelog comments")
	}

	return nil
}
