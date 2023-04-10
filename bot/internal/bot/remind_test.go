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
	"github.com/stretchr/testify/assert"
)

func TestRemind(t *testing.T) {
	cases := []struct {
		description string
		files       []github.PullRequestFile
		// Expect no error if this is empty
		errSubstring     string
		expectedComments []string
		currentComments  []string
	}{
		{
			description: "New docs page",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/server-access/new-feature.mdx",
					Additions: 300,
					Deletions: 0,
					Status:    github.StatusAdded,
				},
			},
			errSubstring: "",
			expectedComments: []string{
				newDocsReminderList,
			},
		},
		{
			description: "Minor docs edits",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/server-access/introduction.mdx",
					Additions: 10,
					Deletions: 2,
					Status:    github.StatusModified,
				},
			},
			errSubstring:     "",
			expectedComments: []string{},
		},
		{
			description: "Major docs edits",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/server-access/introduction.mdx",
					Additions: 1000,
					Deletions: 500,
					Status:    github.StatusModified,
				},
			},
			errSubstring:     "",
			expectedComments: []string{},
		},
		{
			description: "New code file",
			files: []github.PullRequestFile{
				{
					Name:      "api/types/jwt.go",
					Additions: 1000,
					Status:    github.StatusAdded,
				},
			},
			errSubstring:     "",
			expectedComments: []string{},
		},
		{
			description: "Moved docs file",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/server-access/introduction.mdx",
					Additions: 1000,
					Deletions: 500,
					Status:    github.StatusRenamed,
				},
			},
			errSubstring:     "",
			expectedComments: []string{},
		},
		{
			description: "New docs page with existing comment",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/server-access/new-feature.mdx",
					Additions: 300,
					Deletions: 0,
					Status:    github.StatusAdded,
				},
			},
			expectedComments: []string{
				newDocsReminderList,
			},
			currentComments: []string{
				newDocsReminderList,
			},
		},
		{
			description: "New docs partial",
			files: []github.PullRequestFile{
				{
					Name:      "docs/pages/includes/my-partial.mdx",
					Additions: 300,
					Deletions: 0,
					Status:    github.StatusAdded,
				},
			},
			errSubstring:     "",
			expectedComments: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			prNum := 100
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{},
					GitHub:      &fakeGithub{},
				},
			}

			err := b.remind(context.Background(), prNum, c.files)
			if c.errSubstring == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, c.errSubstring)
			}

			m, err := b.c.GitHub.ListComments(context.Background(), "gravitational", "teleport", prNum)
			assert.NoError(t, err)

			actual := make([]string, len(m))
			for i, c := range m {
				actual[i] = c.Body
			}
			assert.ElementsMatch(t, c.expectedComments, actual)
		})
	}
}
