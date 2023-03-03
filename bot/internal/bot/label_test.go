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
		desc               string
		branch             string
		files              []github.PullRequestFile
		latestMajorVersion int
		labels             []string
	}{
		{
			desc:   "code-only",
			branch: "foo",
			files: []github.PullRequestFile{
				{Name: "file.go"},
				{Name: "examples/README.md"},
			},
			labels: []string{string(small)},
		},
		{
			desc:   "docs",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "docs/docs.md",
					Additions: 105,
					Deletions: 10,
				},
			},
			labels: []string{"documentation", string(small)},
		},
		{
			desc:   "helm",
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
			branch: "foo",
			files: []github.PullRequestFile{
				{Name: "docs/docs.md"},
				{Name: "examples/chart/index.html"},
			},
			labels: []string{"documentation", "helm", string(small)},
		},
		{
			desc:   "docs-and-backport",
			branch: "branch/foo",
			files: []github.PullRequestFile{
				{
					Name:      "docs/docs.md",
					Additions: 5555,
					Deletions: 1000,
				},
			},
			labels: []string{"backport", "documentation", string(xlarge)},
		},
		{
			desc:   "web only",
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
			desc:   "code-only with a release version",
			branch: "foo",
			files: []github.PullRequestFile{
				{Name: "file.go"},
				{Name: "examples/README.md"},
			},
			latestMajorVersion: 12,
			labels:             []string{string(small)},
		},
		{
			desc:   "docs with a release version",
			branch: "foo",
			files: []github.PullRequestFile{
				{
					Name:      "docs/docs.md",
					Additions: 105,
					Deletions: 10,
				},
			},
			latestMajorVersion: 12,
			labels: []string{
				"documentation",
				string(small),
				"backport/branch/v12",
				"backport/branch/v11",
				"backport/branch/v10",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Organization: "gravitational",
						Repository:   "teleport",
						Number:       0,
						UnsafeBase:   test.branch,
					},
				},
			}
			labels, err := b.labels(context.Background(), test.files, test.latestMajorVersion)
			require.NoError(t, err)
			require.ElementsMatch(t, labels, test.labels)
		})
	}
}
