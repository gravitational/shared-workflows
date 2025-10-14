package bot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDocsPathsForMissingRedirects(t *testing.T) {
	cases := []struct {
		description       string
		teleportClonePath string
		docsConfig        string
		errorSubstring    string
		number            int
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
			number:         1,
			errorSubstring: "",
		},
		{
			description:       "valid clone path with missing redirect",
			teleportClonePath: "/teleport",
			docsConfig: `{
  "navigation": [],
  "variables": {},
  "redirects": []
}`,
			number:         1,
			errorSubstring: "missing redirects for the following renamed or deleted pages: /database-access/get-started/",
		},
		{
			description:       "invalid config file",
			teleportClonePath: "/teleport",
			docsConfig:        `This file is not JSON.`,
			number:            1,
			errorSubstring:    "docs/config.json: invalid character 'T' looking for beginning of value",
		},
		{
			description:       "invalid config file with PR 0",
			teleportClonePath: "/teleport",
			docsConfig:        `This file is not JSON.`,
			number:            0,
			errorSubstring:    "",
		},
	}

	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			tmpdir := t.TempDir()
			err := os.MkdirAll(filepath.Join(tmpdir, "teleport", "docs"), 0777)
			require.NoError(t, err)

			f, err := os.Create(filepath.Join(tmpdir, "teleport", "docs", "config.json"))
			require.NoError(t, err)

			_, err = f.WriteString(c.docsConfig)
			require.NoError(t, err)

			b := &Bot{
				c: &Config{
					Environment: &env.Environment{
						Number: c.number,
					},
					GitHub: &fakeGithub{
						files: []github.PullRequestFile{
							{
								Name:         "docs/pages/enroll-resources/database-access/get-started.mdx",
								Status:       github.StatusRenamed,
								PreviousName: "docs/pages/database-access/get-started.mdx",
							},
						},
					},
				},
			}

			err = b.CheckDocsPathsForMissingRedirects(context.Background(), filepath.Join(tmpdir, c.teleportClonePath))
			if c.errorSubstring == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, c.errorSubstring)
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
					Status:       github.StatusRenamed,
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
					Status:       github.StatusRenamed,
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
					Status:    github.StatusRemoved,
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
					Status:    github.StatusRemoved,
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
					Status:    github.StatusModified,
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
					Status:       github.StatusRenamed,
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
					Status:       github.StatusRenamed,
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
					Status:       github.StatusRenamed,
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
		{
			description: "renamed 3rd-level includes path with no redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/includes/databases/mysql-certs.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       github.StatusRenamed,
					PreviousName: "docs/pages/includes/databases/mysql.mdx",
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "renamed 4rd-level includes path with no redirects",
			files: github.PullRequestFiles{
				{
					Name:         "docs/pages/connect-your-client/includes/mysql-certs.mdx",
					Additions:    0,
					Deletions:    0,
					Status:       github.StatusRenamed,
					PreviousName: "docs/pages/connect-your-client/includes/mysql.mdx",
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "page replaced with category index",
			files: []github.PullRequestFile{
				{
					Name:   "docs/pages/installation.mdx",
					Status: github.StatusRemoved,
				},
				{
					Name:   "docs/pages/installation/installation.mdx",
					Status: github.StatusAdded,
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "category index replaced with page",
			files: []github.PullRequestFile{
				{
					Name:   "docs/pages/installation.mdx",
					Status: github.StatusAdded,
				},
				{
					Name:   "docs/pages/installation/installation.mdx",
					Status: github.StatusRemoved,
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "page renamed to category index",
			files: []github.PullRequestFile{
				{
					Name:         "docs/pages/installation/installation.mdx",
					PreviousName: "docs/pages/installation.mdx",
					Status:       github.StatusRenamed,
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "category index renamed to page",
			files: []github.PullRequestFile{
				{
					Name:         "docs/pages/installation.mdx",
					PreviousName: "docs/pages/installation/installation.mdx",
					Status:       github.StatusRenamed,
				},
			},
			redirects: []DocsRedirect{},
			expected:  []string{},
		},
		{
			description: "complex rename scenario",
			files: []github.PullRequestFile{
				{
					Name:         "docs/pages/identity-governance/access-lists.mdx",
					PreviousName: "docs/pages/identity-governance/access-lists/guide.mdx",
					Status:       github.StatusRenamed,
				},
				{
					Name:   "docs/pages/identity-governance/access-lists/access-lists.mdx",
					Status: github.StatusRemoved,
				},
			},
			redirects: []DocsRedirect{
				{
					Source:      "/identity-governance/access-lists/guide/",
					Destination: "/identity-governance/access-lists/",
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
