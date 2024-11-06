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
		// Accommodate the Docusaurus logic of generating a route for a
		// section index page, e.g., docs/pages/slug/slug.mdx, at a URL
		// path that consists only of the containing directory, e.g.,
		// /docs/slug/.
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
					Source:      "/databases/",
					Destination: "/enroll-resources/databases/",
					Permanent:   true,
				},
			},
			expected: []string{},
		},
		{
			description: "renamed section index page with inadequate redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/enroll-resources/applications/applications.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       "renamed",
					PreviousName: "docs/pages/applications/applications.mdx",
				},
			},
			redirects: RedirectConfig{
				{
					Source:      "/applications/",
					Destination: "/enroll-resources/applications/applications/",
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

func Test_toURLPath(t *testing.T) {
	cases := []struct {
		description string
		input       string
		expected    string
	}{
		{
			description: "section index page",
			input:       "docs/pages/databases/databases.mdx",
			expected:    "/databases/",
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			assert.Equal(t, c.expected, toURLPath(c.input))
		})
	}
}
