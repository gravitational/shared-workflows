/*
Copyright 2026 Gravitational, Inc.

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
	"text/template"

	"github.com/gravitational/trace"
)

const (
	clTemplate = `
{{- range .Core -}}
* {{.Summary}} [#{{.Number}}]({{.URL}})
{{ end -}}
{{ if .Enterprise }}
Enterprise
{{ range .Enterprise -}}
* {{.Summary}}
{{ end -}}
{{- end -}}
`
	clTemplateNoLink = `
{{- range .Core -}}
* {{.Summary}}
{{ end -}}
{{ if .Enterprise }}
Enterprise
{{ range .Enterprise -}}
* {{.Summary}}
{{ end -}}
{{- end -}}
`
)

var (
	clParsedTmpl       = template.Must(template.New("cl").Parse(clTemplate))
	clParsedTmplNoLink = template.Must(template.New("cl").Parse(clTemplateNoLink))
)

type renderOpts struct {
	changelogs         []changelogInfo
	excludeCorePRLinks bool
}

type renderData struct {
	Core       []changelogInfo
	Enterprise []changelogInfo
}

func renderChangelog(opts renderOpts) (string, error) {
	var buff bytes.Buffer

	coreChangelogs := []changelogInfo{}
	enterpriseChangelogs := []changelogInfo{}

	for _, cl := range opts.changelogs {
		if cl.IsEnterprise {
			enterpriseChangelogs = append(enterpriseChangelogs, cl)
		} else {
			coreChangelogs = append(coreChangelogs, cl)
		}
	}

	if len(coreChangelogs) == 0 && len(enterpriseChangelogs) == 0 {
		return "", nil
	}

	tmpl := clParsedTmpl
	if opts.excludeCorePRLinks {
		tmpl = clParsedTmplNoLink
	}

	if err := tmpl.Execute(&buff, renderData{Core: coreChangelogs, Enterprise: enterpriseChangelogs}); err != nil {
		return "", trace.Wrap(err)
	}

	return buff.String(), nil
}
