package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
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
	Redirects  RedirectConfig
}

type RedirectConfig []Redirect

type Redirect struct {
	Source      string
	Destination string
	Permanent   bool
}

// CheckDocsPathsForMissingRedirects takes the relative path to a
// gravitational/teleport clone. It assumes that there is a file called
// "docs/config.json" at the root of the directory that lists redirects in the
// "redirects" field.
//
// CheckDocsPathsForMissingRedirects checks whether a PR has renamed or deleted
// any docs files and, if so, returns an error if any of these docs files does
// not correspond to a new redirect in the configuration file.
func (b *Bot) CheckDocsPathsForMissingRedirects(ctx context.Context, teleportClonePath string) error {
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

	m := missingRedirectSources(c.Redirects, files)
	if len(m) > 0 {
		return trace.Wrap(fmt.Errorf("docs config at %v is missing redirects for the following renamed or deleted pages: %v", docsConfigPath, strings.Join(m, ",")))
	}

	return nil
}

const docsPrefix = "docs/pages/"

// toURLPath converts a local docs page path to a URL path in the format found
// in a docs redirect configuration.
func toURLPath(p string) string {
	return strings.TrimSuffix(
		strings.ReplaceAll(p, docsPrefix, "/"),
		".mdx",
	) + "/"
}

// missingRedirectSources checks renamed or deleted docs pages in files to
// ensure that there is a corresponding redirect source in conf. For any missing
// redirects, it lists redirect sources that should be in conf.
func missingRedirectSources(conf RedirectConfig, files github.PullRequestFiles) []string {
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