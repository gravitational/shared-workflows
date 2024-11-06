package bot

import (
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/assert"
)

func TestMissingRedirectSources(t *testing.T) {
	cases := []struct {
		description string
		files       github.PullRequestFiles
		redirects   RedirectConfig
		expected    []string
	}{
		{
			description: "renamed docs path with adequate redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/databases/protect-mysql.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       "renamed",
					PreviousName: "docs/pages/databases/mysql.mdx",
				},
			},
			redirects: RedirectConfig{
				{
					Source:      "/databases/mysql/",
					Destination: "/databases/protect-mysql/",
					Permanent:   true,
				},
			},
			expected: []string{},
		},
		{
			description: "renamed docs path with missing redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/databases/protect-mysql.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       "renamed",
					PreviousName: "docs/pages/databases/mysql.mdx",
				},
			},
			redirects: RedirectConfig{},
			expected:  []string{"/databases/mysql/"},
		},
		{
			description: "deleted docs path with adequate redirects",
			files: github.PullRequestFiles{
				{
					Name:      "docs/pages/databases/mysql.mdx",
					Additions: 0,
					Deletions: 0,
					Status:    "removed",
				},
			},
			redirects: RedirectConfig{
				{
					Source:      "/databases/mysql/",
					Destination: "/databases/protect-mysql/",
					Permanent:   true,
				},
			}, expected: []string{},
		},
		{
			description: "deleted docs path with missing redirects",
			files: github.PullRequestFiles{
				{
					Name:      "docs/pages/databases/mysql.mdx",
					Additions: 0,
					Deletions: 200,
					Status:    "removed",
				},
			},
			redirects: RedirectConfig{},
			expected:  []string{"/databases/mysql/"},
		},
		{
			description: "modified docs page",
			files: github.PullRequestFiles{
				{
					Name:      "docs/pages/databases/mysql.mdx",
					Additions: 50,
					Deletions: 15,
					Status:    "modified",
				},
			},
			redirects: RedirectConfig{},
			expected:  []string{},
		},
		{
			description: "renamed docs path with nil redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/databases/protect-mysql.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       "renamed",
					PreviousName: "docs/pages/databases/mysql.mdx",
				},
			},
			redirects: nil,
			expected:  []string{"/databases/mysql/"},
		},
		{
			description: "redirects with nil files",
			files:       nil,
			redirects: RedirectConfig{
				{
					Source:      "/databases/mysql/",
					Destination: "/databases/protect-mysql/",
					Permanent:   true,
				},
			}, expected: []string{},
		},
		// TODO: Once we use the Docusaurus-native configuration syntax,
		// rather than migrate the gravitational/docs configuration
		// syntax, the destination will take the form,
		// "/enroll-resources/databases/". I.e., the duplicate path
		// segment is removed.
		{
			description: "renamed section index page with adequate redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/enroll-resources/databases/databases.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       "renamed",
					PreviousName: "docs/pages/databases/databases.mdx",
				},
			},
			redirects: RedirectConfig{
				{
					Source:      "/databases/databases/",
					Destination: "/enroll-resources/databases/databases/",
					Permanent:   true,
				},
			},
			expected: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			assert.Equal(t, c.expected, missingRedirectSources(c.redirects, c.files))
		})
	}
}
