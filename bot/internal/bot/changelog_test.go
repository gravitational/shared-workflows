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
	"fmt"
	"strings"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/require"
)

func TestChangelog(t *testing.T) {
	t.Run("pass-no-changelog-label", func(t *testing.T) {
		b, ctx := buildTestingFixtures()
		b.c.GitHub.(*fakeGithub).pull.UnsafeLabels = []string{NoChangelogLabel}

		err := b.Changelog(ctx)
		require.True(t, err == nil)
	})
}

func TestGetChangelogEntry(t *testing.T) {
	tests := []struct {
		desc        string
		body        string
		shouldError bool
		expected    string
	}{
		{
			desc:        "pass-simple",
			body:        strings.Join([]string{"some typical PR entry", fmt.Sprintf("%schangelog entry", ChangelogPrefix)}, "\n"),
			shouldError: false,
			expected:    "changelog entry",
		},
		{
			desc:        "pass-case-invariant",
			body:        strings.Join([]string{"some typical PR entry", fmt.Sprintf("%schangelog entry", strings.ToUpper(ChangelogPrefix))}, "\n"),
			shouldError: false,
			expected:    "changelog entry",
		},
		{
			desc:        "pass-prefix-in-changelog-entry",
			body:        strings.Join([]string{"some typical PR entry", strings.Repeat(ChangelogPrefix, 5)}, "\n"),
			shouldError: false,
			expected:    strings.Repeat(ChangelogPrefix, 4),
		},
		{
			desc:        "pass-only-changelog-in-body",
			body:        fmt.Sprintf("%schangelog entry", ChangelogPrefix),
			shouldError: false,
			expected:    "changelog entry",
		},
		{
			desc:        "fail-if-no-body",
			body:        "",
			shouldError: true,
		},
		{
			desc:        "fail-if-no-entry",
			body:        "some typical PR entry",
			shouldError: true,
		},
		{
			desc:        "fail-if-multiple-entries",
			body:        strings.Join([]string{ChangelogPrefix, ChangelogPrefix, ChangelogPrefix}, "\n"),
			shouldError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b, ctx := buildTestingFixtures()

			changelogEntry, err := b.getChangelogEntry(ctx, test.body)
			require.Equal(t, test.shouldError, err != nil)
			if !test.shouldError {
				require.Equal(t, test.expected, changelogEntry)
			}
		})
	}
}

func TestValidateGetChangelogEntry(t *testing.T) {
	tests := []struct {
		desc        string
		entry       string
		shouldError bool
	}{
		{
			desc:        "pass-simple",
			entry:       "Changelog entry",
			shouldError: false,
		},
		{
			desc:        "pass-markdown-single-line-code-block",
			entry:       "Changelog `entry`",
			shouldError: false,
		},
		{
			desc:        "fail-empty",
			entry:       "",
			shouldError: true,
		},
		{
			desc:        "fail-whitespace",
			entry:       " \t ",
			shouldError: true,
		},
		{
			desc:        "fail-non-alphabetical-starting-character",
			entry:       "1234Changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-lowercase-starting-character",
			entry:       "changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-trailing-whitespace",
			entry:       "Changelog entry ",
			shouldError: true,
		},
		{
			desc:        "fail-leading-whitespace",
			entry:       " Changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-refers-to-backport",
			entry:       "Backport of #1234",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-punctuation",
			entry:       "Changelog entry.",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-link",
			entry:       "Changelog [entry](https://some-link.com).",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-image",
			entry:       "Changelog ![entry](https://some-link.com).",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-header",
			entry:       "## Changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-ordered-list",
			entry:       "1. Changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-unordered-list",
			entry:       "- Changelog entry",
			shouldError: true,
		},
		{
			desc:        "fail-ends-with-markdown-multiline-code-block",
			entry:       "Changelog entry ```",
			shouldError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b, ctx := buildTestingFixtures()

			err := b.validateChangelogEntry(ctx, test.entry)
			require.Equal(t, test.shouldError, err != nil)
		})
	}
}

func TestLogFailedCheck(t *testing.T) {
	t.Run("fail-contains-passed-message", func(t *testing.T) {
		b, ctx := buildTestingFixtures()

		err := b.logFailedCheck(ctx, "error %s", "message")
		require.ErrorContains(t, err, "error message")
	})
}

func TestDeleteComments(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		b, ctx := buildTestingFixtures()

		err := b.deleteComments(ctx)
		require.True(t, err == nil)
	})
}

func buildTestingFixtures() (*Bot, context.Context) {
	return &Bot{
		c: &Config{
			Environment: &env.Environment{
				Organization: "foo",
				Author:       "9",
				Repository:   "bar",
				Number:       0,
				UnsafeBase:   "branch/v8",
				UnsafeHead:   "fix",
			},
			GitHub: &fakeGithub{
				comments: []github.Comment{
					{
						Author: "foo@bar.com",
						Body:   "PR comment body",
						ID:     1234,
					},
				},
			},
		},
	}, context.Background()
}
