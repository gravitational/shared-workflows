/*
 * Teleport
 * Copyright (C) 2024  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/gravitational/shared-workflows/libs/gh"
	"github.com/gravitational/trace"
)

const (
	// timeNow is a convenience variable to signal that search should include PRs up to current time
	timeNow = ""
)

// changelogInfo is used for the changelog template format.
type changelogInfo struct {
	// Summary is the changelog summary extracted from a PR
	Summary string
	Number  int
	URL     string
}

const (
	ossCLTemplate = `
{{- range . -}}
* {{.Summary}} [#{{.Number}}]({{.URL}})
{{ end -}}
`
	entCLTemplate = `
{{- range . -}}
* {{.Summary}}
{{ end -}}
`
)

var (
	// clPattern will match a changelog format with the summary as a subgroup.
	// e.g. will match a line "changelog: this is a changelog" with subgroup "this is a changelog".
	clPattern = regexp.MustCompile(`[Cc]hangelog: +(.*)`)

	ossCLParsedTmpl = template.Must(template.New("oss cl").Parse(ossCLTemplate))
	entCLParsedTmpl = template.Must(template.New("enterprise cl").Parse(entCLTemplate))
)

type changelogGenerator struct {
	isEnt bool
	dir   string
}

// generateChangelog will pull a PRs from branch between two points in time and generate a changelog from them.
func (c *changelogGenerator) generateChangelog(branch, fromTime, toTime string) (string, error) {
	// Search github for changelog pull requests
	prs, err := gh.ListChangelogPullRequests(
		context.Background(),
		c.dir,
		&gh.ListChangelogPullRequestsOpts{
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
func (c *changelogGenerator) toChangelog(prs []gh.ChangelogPR) (string, error) {
	var clList []changelogInfo
	for _, pr := range prs {
		clList = append(clList, newChangelogInfoFromPR(pr))
	}

	var tmpl *template.Template
	if c.isEnt {
		tmpl = entCLParsedTmpl
	} else {
		tmpl = ossCLParsedTmpl
	}

	var buff bytes.Buffer
	if err := tmpl.Execute(&buff, clList); err != nil {
		return "", trace.Wrap(err)
	}

	return buff.String(), nil
}

// convertPRToChangelog will convert the list of PRs to a nicer format.
func newChangelogInfoFromPR(pr gh.ChangelogPR) changelogInfo {
	found, clSummary := findChangelog(pr.Body)
	if !found {
		// Pull out title and indicate no changelog found
		clSummary = fmt.Sprintf("NOCL: %s", pr.Title)
	}
	return changelogInfo{
		Summary: prettierSummary(clSummary),
		Number:  pr.Number,
		URL:     pr.URL,
	}
}

// findChangelog will parse a body of a PR to find a changelog.
func findChangelog(commentBody string) (found bool, summary string) {
	// If a match is found then we should get a non empty slice
	// 0 index will be the whole match including "changelog: *"
	// 1 index will be the subgroup match which does not include "changelog: "
	m := clPattern.FindStringSubmatch(commentBody)
	if len(m) > 1 {
		return true, m[1]
	}
	return false, ""
}

func prettierSummary(cl string) string {
	cl = strings.TrimSpace(cl)
	if !strings.HasSuffix(cl, ".") {
		cl += "."
	}
	return cl
}
