/*
Copyright 2024 Gravitational, Inc.

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

package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/trace"
)

// changelogInfo is used for the changelog template format.
type changelogInfo struct {
	// Summary is the changelog summary extracted from a PR
	Summary string
	Number  int
	URL     string
}

const (
	clTemplate = `
{{- range . -}}
* {{.Summary}} [#{{.Number}}]({{.URL}})
{{ end -}}
`
	clTemplateNoLink = `
{{- range . -}}
* {{.Summary}}
{{ end -}}
`
)

var (
	// clPattern will match a changelog format with the summary as a subgroup.
	// e.g. will match a line "changelog: this is a changelog" with subgroup "this is a changelog".
	clPattern = regexp.MustCompile(`[Cc]hangelog: +(.*)`)

	clParsedTmpl       = template.Must(template.New("cl").Parse(clTemplate))
	clParsedTmplNoLink = template.Must(template.New("cl").Parse(clTemplateNoLink))
)

type changelogGenerator struct {
	repo           string
	ghclient       *github.Client
	excludePRLinks bool
}

// generateChangelog will pull a PRs from branch between two points in time and generate a changelog from them.
func (c *changelogGenerator) generateChangelog(ctx context.Context, branch string, fromTime, toTime time.Time) (string, error) {
	// Search github for changelog pull requests
	prs, err := c.ghclient.ListChangelogPullRequests(
		ctx,
		"gravitational",
		c.repo,
		&github.ListChangelogPullRequestsOpts{
			Branch:   branch,
			FromDate: fromTime,
			ToDate:   toTime,
		},
	)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return c.toChangelog(prs)
}

// toChangelog will take the output from the search and format it into a changelog.
func (c *changelogGenerator) toChangelog(prs []github.ChangelogPR) (string, error) {
	var clList []changelogInfo
	for _, pr := range prs {
		clList = append(clList, newChangelogInfoFromPR(pr)...)
	}

	var buff bytes.Buffer
	tmpl := clParsedTmpl
	if c.excludePRLinks {
		tmpl = clParsedTmplNoLink
	}
	if err := tmpl.Execute(&buff, clList); err != nil {
		return "", trace.Wrap(err)
	}

	return buff.String(), nil
}

// convertPRToChangelog will convert the list of PRs to a nicer format.
func newChangelogInfoFromPR(pr github.ChangelogPR) []changelogInfo {
	found, summaries := findChangelogs(pr.Body)
	if !found {
		// Pull out title and indicate no changelog found
		return []changelogInfo{
			{
				Summary: fmt.Sprintf("NOCL: %s", pr.Title),
				Number:  pr.Number,
				URL:     pr.URL,
			},
		}
	}

	cls := []changelogInfo{}
	for _, clSummary := range summaries {
		cls = append(cls, changelogInfo{
			Summary: prettierSummary(clSummary),
			Number:  pr.Number,
			URL:     pr.URL,
		})
	}

	return cls
}

// findChangelog will parse a body of a PR to find a changelog.
func findChangelogs(commentBody string) (found bool, summaries []string) {
	// If a match is found then we should get a non empty slice
	// 0 index will be the whole match including "changelog: *"
	// 1 index will be the subgroup match which does not include "changelog: "
	m := clPattern.FindAllStringSubmatch(commentBody, -1)
	if len(m) < 1 {
		return false, nil
	}

	sum := []string{}

	for _, v := range m {
		sum = append(sum, v[1])
	}
	return true, sum
}

func prettierSummary(cl string) string {
	// Clean whitespace and add a period at end
	cl = strings.TrimSpace(cl)
	if !strings.HasSuffix(cl, ".") {
		cl += "."
	}

	// Uppercase first letter
	r := []rune(cl)
	r[0] = unicode.ToUpper(r[0])
	cl = string(r)

	return cl
}
