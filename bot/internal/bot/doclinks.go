package bot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"text/template"

	"github.com/gravitational/trace"
	"github.com/spf13/afero"
)

type staleDocURL struct {
	Text string
	Line int
}

// CheckDocsURLs opens each file changed by a pull request and checks whether
// any URL paths for the Teleport documentation site within the file are
// incorrect. An incorrect URL path is one that does not correspond to either:
//   - a docs page in the docs/pages directory within teleportClonePath
//   - the source of a redirect named in the docs configuration file at
//     docs/config.json within teleportClonePath
func (b *Bot) CheckDocsURLs(ctx context.Context, teleportClonePath string) error {
	if teleportClonePath == "" {
		return trace.Wrap(errors.New("unable to load Teleport documentation config with an empty path"))
	}

	docsConfigPath := path.Join(teleportClonePath, "docs", "config.json")
	f, err := os.Open(docsConfigPath)
	if err != nil {
		return trace.Wrap(fmt.Errorf("unable to open docs config file at %v: %v", docsConfigPath, err))
	}
	defer f.Close()

	var c DocsConfig
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return trace.Wrap(fmt.Errorf("unable to load redirect configuration from %v: %v", docsConfigPath, err))
	}

	files, err := b.c.GitHub.ListFiles(ctx, b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(fmt.Errorf("unable to fetch files for PR %v: %v", b.c.Environment.Number, err))
	}

	data := make(staleDocsURLData)
	for _, f := range files {
		l, err := os.Open(f.Name)
		if err != nil {
			return trace.Wrap(fmt.Errorf("unable to open PR file %v: %v", f.Name, err))
		}
		defer l.Close()

		stale := staleDocsURLs(afero.NewOsFs(), c.Redirects, l, teleportClonePath)
		if len(stale) == 0 {
			continue
		}
		data[f.Name] = stale
	}

	if len(data) != 0 {
		return trace.Wrap(fmt.Errorf("found incorrect documentation URLs in the following files:\n%v", data))
	}

	return nil
}

type staleDocsURLData map[string][]staleDocURL

const staleURLDataTempl = `{{ range $key, $val := . }}{{ $key }}:
{{ range $val }}  - line {{ .Line }}: {{ .Text }}
{{ end }}
{{ end }}`

func (s staleDocsURLData) String() string {
	var buf bytes.Buffer
	template.Must(template.New("").Parse(staleURLDataTempl)).Execute(&buf, s)
	return buf.String()
}

var docURLRegexp = regexp.MustCompile(`goteleport.com(/([a-zA-Z0-9\._-]+/?)+)`)

// docURLPathToFilePath returns the name of the docs file path, relative to the
// root of a gravitational/teleport clone directory, that corresponds to the
// provided URL path, which is a valid redirect source.
//
// For example, "/admin-guides/database-access/introduction/" would map to
// "docs/pages/admin-guides/database-access/introduction.mdx".
func docURLPathToFilePath(urlpath string) string {
	return docsPrefix + strings.TrimSuffix(urlpath, "/") + ".mdx"
}

// staleDocsURLs examines file to detect stale docs site URLs. A style docs site
// URL is a URL of a docs page that neither corresponds to a file in fs nor a
// redirect source in conf.
//
// staleDocsURLs assumes that docs paths are in fs and relative to
// teleportClonePath.
func staleDocsURLs(fs afero.Fs, conf RedirectConfig, file io.Reader, teleportClonePath string) []staleDocURL {
	sources := make(map[string]struct{})
	for _, s := range conf {
		sources[s.Source] = struct{}{}
	}

	res := []staleDocURL{}
	sc := bufio.NewScanner(file)
	sc.Split(bufio.ScanLines)
	var line int
	for sc.Scan() {
		line++
		m := docURLRegexp.FindAllStringSubmatch(sc.Text(), -1)
		for _, a := range m {
			urlpath := a[1]
			// Redirect sources include a trailing "/".
			if !strings.HasSuffix(urlpath, "/") {
				urlpath += "/"
			}

			// While goteleport.com/docs URLs include the /docs/
			// path segment, redirect sources do not.
			urlpath = strings.TrimPrefix(urlpath, "/docs")

			_, ok := sources[urlpath]
			_, err := fs.Stat(path.Join(teleportClonePath, docURLPathToFilePath(urlpath)))
			if ok || err == nil {
				continue
			}

			res = append(res, staleDocURL{
				Text: a[0],
				Line: line,
			})
		}
	}

	return res
}
