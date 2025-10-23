/*
Copyright 2025 Gravitational, Inc.

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

	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
)

func TestReconcileComment(t *testing.T) {
	tests := []struct {
		desc             string
		existingComments []github.Comment
		body             string
		expectCreate     bool
		expectEditID     int64
	}{
		{
			desc:             "no existing comments - should create",
			existingComments: []github.Comment{},
			body:             "# 📦 Bundle Size Report\n\nTest report content",
			expectCreate:     true,
		},
		{
			desc: "existing bot comment - should update",
			existingComments: []github.Comment{
				{
					ID:   123,
					Body: "# 📦 Bundle Size Report\n\nOld report content",
				},
			},
			body:         "# 📦 Bundle Size Report\n\nNew report content",
			expectCreate: false,
			expectEditID: 123,
		},
		{
			desc: "existing non-bot comments - should create",
			existingComments: []github.Comment{
				{
					ID:   456,
					Body: "This is a regular comment",
				},
				{
					ID:   789,
					Body: "Another regular comment",
				},
			},
			body:         "# 📦 Bundle Size Report\n\nTest report content",
			expectCreate: true,
		},
		{
			desc: "multiple comments with bot comment - should update bot comment",
			existingComments: []github.Comment{
				{
					ID:   100,
					Body: "Regular comment before",
				},
				{
					ID:   200,
					Body: "# 📦 Bundle Size Report\n\nExisting bot report",
				},
				{
					ID:   300,
					Body: "Regular comment after",
				},
			},
			body:         "# 📦 Bundle Size Report\n\nUpdated bot report",
			expectCreate: false,
			expectEditID: 200,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gh := &fakeGithub{
				comments: test.existingComments,
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

			err := bot.reconcileComment(context.Background(), test.body)

			require.NoError(t, err)

			if test.expectCreate {
				require.Len(t, gh.comments, len(test.existingComments)+1)

				lastComment := gh.comments[len(gh.comments)-1]

				require.Equal(t, test.body, lastComment.Body)
			} else {
				require.Len(t, gh.comments, len(test.existingComments))

				for _, comment := range gh.comments {
					if comment.ID == test.expectEditID {
						require.Equal(t, test.body, comment.Body)

						break
					}
				}
			}
		})
	}
}
