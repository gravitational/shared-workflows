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
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/trace"
)

// entry is a single changelog entry, one line of the rendered changelog.
type entry struct {
	// Summary is the changelog summary extracted from a PR
	Summary string
	Number  int
	URL     string
}

var (
	// clPattern matches a "changelog: <summary>" line, capturing the summary.
	clPattern = regexp.MustCompile(`[Cc]hangelog: +(.*)`)

	// tmplLinks renders entries with a link to the PR; tmplNoLinks without.
	tmplLinks = template.Must(template.New("cl").Parse(`
{{- range . -}}
* {{.Summary}} [#{{.Number}}]({{.URL}})
{{ end -}}
`))
	tmplNoLinks = template.Must(template.New("cl").Parse(`
{{- range . -}}
* {{.Summary}}
{{ end -}}
`))
)

// org is the GitHub organization the repos belong to.
const org = "gravitational"

type generator struct {
	repo string
	gh   *github.Client
	tmpl *template.Template
}

// generate fetches the given PRs and renders a changelog from them.
func (g *generator) generate(ctx context.Context, prNumbers []int) (string, error) {
	prs, err := g.gh.PullRequests(ctx, org, g.repo, prNumbers)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return g.render(prs)
}

// render formats the PRs' changelog entries into a changelog.
func (g *generator) render(prs []github.PullRequest) (string, error) {
	var entries []entry
	for _, pr := range prs {
		entries = append(entries, entriesFromPR(pr)...)
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, entries); err != nil {
		return "", trace.Wrap(err)
	}

	return buf.String(), nil
}

// entriesFromPR extracts changelog entries from a PR's body.
func entriesFromPR(pr github.PullRequest) []entry {
	var entries []entry
	for _, m := range clPattern.FindAllStringSubmatch(pr.Body, -1) {
		entries = append(entries, entry{
			Summary: formatSummary(m[1]),
			Number:  pr.Number,
			URL:     pr.URL,
		})
	}
	return entries
}

func formatSummary(s string) string {
	s = strings.TrimSpace(s)
	// Appending the period first guarantees s is non-empty for r[0] below.
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
