package bot

import (
	"path"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestStaleDocsURLs(t *testing.T) {
	cases := []struct {
		description   string
		sourceFile    string
		docsFilenames []string
		redirects     RedirectConfig
		expected      []staleDocURL
	}{
		{
			description: "no errors, no redirects",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases`,
			docsFilenames: []string{
				"docs/pages/enroll-databases.mdx",
			},
			redirects: RedirectConfig{},
			expected:  []staleDocURL{},
		},
		{
			description: "one error, no redirects",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases`,
			docsFilenames: []string{
				"docs/pages/enroll-dbs.mdx",
			},
			redirects: RedirectConfig{},
			expected: []staleDocURL{
				{
					Text: "goteleport.com/docs/enroll-databases",
					Line: 1,
				},
			},
		},
		{
			description: "no errors, one redirect",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases`,
			docsFilenames: []string{
				"docs/pages/enroll-dbs.mdx",
			},
			redirects: RedirectConfig{
				{
					Source:      "/enroll-databases/",
					Destination: "/enroll-dbs/",
					Permanent:   true,
				},
			},
			expected: []staleDocURL{},
		},
		{
			description: "error on line 3",
			sourceFile: `This is a paragraph.

This is a link to a [docs page](https://www.goteleport.com/docs/enroll-databases).`,
			docsFilenames: []string{
				"docs/pages/enroll-dbs.mdx",
			},
			redirects: RedirectConfig{},
			expected: []staleDocURL{
				{
					Text: "goteleport.com/docs/enroll-databases",
					Line: 3,
				},
			},
		},
		{
			description: "no subdomain in docs link, no error",
			sourceFile:  `https://goteleport.com/docs/enroll-databases`,
			docsFilenames: []string{
				"docs/pages/enroll-databases.mdx",
			},
			redirects: RedirectConfig{},
			expected:  []staleDocURL{},
		},
		{
			description: "query string in docs link, no error",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases?scope="enterprise"`,
			docsFilenames: []string{
				"docs/pages/enroll-databases.mdx",
			},
			redirects: RedirectConfig{},
			expected:  []staleDocURL{},
		},
		{
			description: "fragment in docs link, no error",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases#step14-install-teleport`,
			docsFilenames: []string{
				"docs/pages/enroll-databases.mdx",
			},
			redirects: RedirectConfig{},
			expected:  []staleDocURL{},
		},
		{
			description: "trailing slash in docs link, no error",
			sourceFile:  `https://www.goteleport.com/docs/enroll-databases/`,
			docsFilenames: []string{
				"docs/pages/enroll-databases.mdx",
			},
			redirects: RedirectConfig{},
			expected:  []staleDocURL{},
		},
	}

	for _, c := range cases {
		fs := afero.NewMemMapFs()
		for _, n := range c.docsFilenames {
			_, err := fs.Create(path.Join("docs", n))
			assert.NoError(t, err)
		}

		t.Run(c.description, func(t *testing.T) {
			urls := staleDocsURLs(fs, c.redirects, strings.NewReader(c.sourceFile), "docs")
			assert.Equal(t, c.expected, urls)
		})
	}
}

func TestString(t *testing.T) {
	expected := `my/file/1.go:
  - line 1: https://goteleport.com/docs/page1
  - line 10: https://goteleport.com/docs/page2
  - line 5: https://goteleport.com/docs/page3

my/file/2.go:
  - line 304: https://goteleport.com/docs/page1
  - line 1003: https://goteleport.com/docs/page2

my/file/3.go:
  - line 19: https://goteleport.com/docs/page1
  - line 253: https://goteleport.com/docs/page2

`

	data := staleDocsURLData{
		"my/file/1.go": []staleDocURL{
			{Line: 1, Text: "https://goteleport.com/docs/page1"},
			{Line: 10, Text: "https://goteleport.com/docs/page2"},
			{Line: 5, Text: "https://goteleport.com/docs/page3"},
		},

		"my/file/2.go": []staleDocURL{
			{Line: 304, Text: "https://goteleport.com/docs/page1"},
			{Line: 1003, Text: "https://goteleport.com/docs/page2"},
		},

		"my/file/3.go": []staleDocURL{
			{Line: 19, Text: "https://goteleport.com/docs/page1"},
			{Line: 253, Text: "https://goteleport.com/docs/page2"},
		},
	}

	assert.Equal(t, expected, data.String())
}