package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

// DocsConfig represents the structure of the config.json file found in the docs
// directory of a Teleport repo. We only use Redirects, so other fields have the
// "any" type.
type DocsConfig struct {
	Navigation any
	Variables  any
	Redirects  []DocsRedirect
}

type DocsRedirect struct {
	Source      string
	Destination string
	Permanent   bool
}

// CheckDocsPathsForMissingRedirects checks whether a PR has renamed or deleted
// any docs files and, if so, returns an error if any of these docs files does
// not correspond to a new redirect in the docs configuration file.
//
// teleportClonePath is a relative path to a gravitational/teleport clone. It is
// assumed that there is a file called docs/config.json at the root of the
// directory that lists redirects in the redirects field.
func (b *Bot) CheckDocsPathsForMissingRedirects(ctx context.Context, teleportClonePath string) error {
	// The event is not a pull request, so don't check PR files.
	if b.c.Environment.Number == 0 {
		return nil
	}

	if teleportClonePath == "" {
		return trace.BadParameter("unable to load Teleport documentation config with an empty path")
	}

	docsConfigPath := filepath.Join(teleportClonePath, "docs", "config.json")
	f, err := os.Open(docsConfigPath)
	if err != nil {
		return trace.BadParameter("unable to load Teleport documentation config at %v: %v", teleportClonePath, err)
	}
	defer f.Close()

	var c DocsConfig
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return trace.BadParameter("unable to load redirect configuration from %v: %v", docsConfigPath, err)
	}

	files, err := b.c.GitHub.ListFiles(ctx, b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err, "unable to fetch files for PR %v: %v", b.c.Environment.Number)
	}

	m := missingRedirectSources(c.Redirects, files)
	if len(m) > 0 {
		return trace.Errorf("docs config at %v is missing redirects for the following renamed or deleted pages: %v", docsConfigPath, strings.Join(m, ","))
	}

	return nil
}

var docsPrefix = filepath.Join("docs", "pages")

// toURLPath converts a local docs page path to a URL path in the format found
// in a docs redirect configuration.
//
// If the name of the file is the same as the name of the containing directory,
// the page is a category index page. This means that the route Docusaurus
// generates for the page is the path to the containing directory, omitting the
// final path segment.
//
// See:
// https://docusaurus.io/docs/sidebar/autogenerated#category-index-convention
func toURLPath(p string) string {
	// Redirect sources do not include the docs content directory path.
	redir := strings.TrimPrefix(p, docsPrefix)
	ext := filepath.Ext(redir)
	// Redirect sources do not contain file extensions.
	redir = strings.TrimSuffix(redir, ext)

	// The file is a category index page, so return the path to its
	// containing directory.
	if strings.HasSuffix(filepath.Dir(redir), filepath.Base(redir)) {
		redir = filepath.Dir(redir)
	}

	// Add a trailing slash to the final redirect path
	return redir + "/"
}

// missingRedirectSources checks renamed or deleted docs pages in files to
// ensure that there is a corresponding redirect source in conf. For any missing
// redirects, it lists redirect sources that should be in conf.
func missingRedirectSources(conf []DocsRedirect, files github.PullRequestFiles) []string {
	sources := make(map[string]struct{})
	for _, s := range conf {
		sources[s.Source] = struct{}{}
	}

	res := []string{}
	for _, f := range files {
		if !strings.HasPrefix(f.Name, docsPrefix) {
			continue
		}

		switch f.Status {
		case "renamed":
			p := toURLPath(f.PreviousName)
			if _, ok := sources[p]; !ok {
				res = append(res, p)
			}
		case "removed":
			p := toURLPath(f.Name)
			if _, ok := sources[p]; !ok {
				res = append(res, p)
			}
		}
	}
	return res
}
