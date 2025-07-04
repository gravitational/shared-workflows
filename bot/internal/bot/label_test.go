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

	"github.com/stretchr/testify/require"
)

// TestLabel checks that labels are correctly applied to a Pull Request.
func TestLabel(t *testing.T) {
	tests := []struct {
		desc   string
		repo   string
		branch string
		files  []github.PullRequestFile
		labels []string
	}{
		{
			desc:   "code-only",
			repo:   "teleport",
			branch: "foo",
			files: []github.PullRequestFile{
				{Name: "file.go"},
				{Name: "examples/README.md"},
			},
			labels: []string{string(small)},
		},
		{
			desc:   "docs",
			repo:   "teleport",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "docs/docs.md",
					Additions: 105,
					Deletions: 10,
				},
			},
			labels: []string{"documentation", NoChangelogLabel, string(small)},
		},
		{
			desc:   "helm",
			repo:   "teleport",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "examples/chart/index.html",
					Additions: 500,
				},
			},
			labels: []string{"helm", string(medium)},
		},
		{
			desc:   "docs-and-helm",
			repo:   "teleport",
			branch: "foo",
			files: []github.PullRequestFile{
				{Name: "docs/docs.md"},
				{Name: "examples/chart/index.html"},
			},
			labels: []string{"documentation", "helm", string(small)},
		},
		{
			desc:   "docs-and-backport",
			repo:   "teleport",
			branch: "branch/foo",
			files: []github.PullRequestFile{
				{
					Name:      "docs/docs.md",
					Additions: 5555,
					Deletions: 1000,
				},
			},
			labels: []string{"backport", "documentation", "no-changelog", string(xlarge)},
		},
		{
			desc:   "web only",
			repo:   "teleport",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "web/packages/design/package.json",
					Additions: 1,
				},
			},
			labels: []string{"ui", string(small)},
		},
		{
			desc:   "teleport.e",
			repo:   "teleport.e",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "lib/devicetrust/file.go",
					Additions: 1,
				},
				{
					Name:      "lib/okta/file.go",
					Additions: 1,
				},
			},
			labels: []string{"device-trust", "application-access", string(small)},
		},
		{
			desc:   "labels for repo don't exist",
			repo:   "doesnt-exist",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "lib/devicetrust/file.go",
					Additions: 1,
				},
				{
					Name:      "lib/okta/file.go",
					Additions: 1,
				},
			},
			labels: []string{string(small)},
		},
		{
			desc:   "labels for the cloud repo",
			repo:   "cloud",
			branch: "master",
			files: []github.PullRequestFile{
				{
					Name:      "rfd/0000_foo.md",
					Additions: 1,
				},
				{
					Name:      "db/salescenter/migrations/218390213.down.sql",
					Additions: 1,
				},
				{
					Name:      "deploy/fluxcd/src/platform/values.yaml",
					Additions: 1,
				},
			},
			labels: []string{string(small), "rfd", "salescenter", "db-migration", "CICD", "platform"},
		},
		{
			desc:   "labels for a cloud deploy branch",
			repo:   "cloud",
			branch: "staging",
			files: []github.PullRequestFile{
				{
					Name:      "rfd/0000_foo.md",
					Additions: 1,
				},
			},
			labels: []string{"rfd"},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Organization: "foo",
						Repository:   test.repo,
						Number:       0,
						UnsafeBase:   test.branch,
					},
				},
			}
			labels, err := b.labels(context.Background(), test.files)
			require.NoError(t, err)
			require.ElementsMatch(t, labels, test.labels)
		})
	}
}
