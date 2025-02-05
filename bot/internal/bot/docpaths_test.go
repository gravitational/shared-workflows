package bot

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestCheckDocsPathsForMissingRedirects(t *testing.T) {
	cases := []struct {
		description       string
		teleportClonePath string
		docsConfig        string
		expected          string
	}{
		{
			description:       "valid clone path with no error",
			teleportClonePath: "/teleport",
			docsConfig: `{
  "navigation": [],
  "variables": {},
  "redirects": [
      {
      	  "source": "/database-access/get-started/",
      	  "destination": "/enroll-resources/database-access/get-started/",
      	  "permanent": true
      }
  ]
}`,
			expected: "",
		},
		{
			description:       "valid clone path with missing redirect",
			teleportClonePath: "/teleport",
			docsConfig: `{
  "navigation": [],
  "variables": {},
  "redirects": []
}`,
			expected: "docs config at /teleport/docs/config.json is missing redirects for the following renamed or deleted pages: /database-access/get-started/",
		},
		{
			description:       "invalid clone path",
			teleportClonePath: "/tele",
			expected:          "unable to load Teleport documentation config at /tele: open /tele/docs/config.json: file does not exist",
		},
		{
			description:       "invalid config file",
			teleportClonePath: "/teleport",
			docsConfig:        `This file is not JSON.`,
			expected:          "unable to load redirect configuration from /teleport/docs/config.json: invalid character 'T' looking for beginning of value",
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			afero.WriteFile(fs, "/teleport/docs/config.json", []byte(c.docsConfig), 0777)
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{},
					GitHub: &fakeGithub{
						files: []github.PullRequestFile{
							{
								Name:         "docs/pages/enroll-resources/database-access/get-started.mdx",
								Status:       "renamed",
								PreviousName: "docs/pages/database-access/get-started.mdx",
							},
						},
					},
				},
			}

			err := b.CheckDocsPathsForMissingRedirects(fs, context.Background(), c.teleportClonePath)

			if c.expected == "" {
				assert.NoError(t, err)
				return
			}
			assert.EqualError(t, err, c.expected)
		})
	}
}

func TestMissingRedirectSources(t *testing.T) {
	cases := []struct {
		description string
		files       github.PullRequestFiles
		redirects   []DocsRedirect
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
			redirects: []DocsRedirect{
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
			redirects: []DocsRedirect{},
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
			redirects: []DocsRedirect{
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
			redirects: []DocsRedirect{},
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
			redirects: []DocsRedirect{},
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
			redirects: []DocsRedirect{
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
			redirects: []DocsRedirect{
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
			redirects: []DocsRedirect{
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
