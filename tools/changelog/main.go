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
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/libs/git"
	"github.com/gravitational/shared-workflows/libs/github"
)

// config holds the tool's resolved command line options.
type config struct {
	// dir is the directory of the teleport repo.
	dir string
	// repoName is the GitHub repo to read changelog entries from.
	repoName string
	// baseBranch is the release ref to generate the changelog for.
	baseBranch string
	// baseTag is the release ref to generate the changelog from.
	baseTag string
	// submodule reports whether the e enterprise repo is a submodule.
	submodule bool
	// prLinks reports whether to link entries to their pull requests.
	prLinks bool
	// prNumberCutoff is the date from which a commit subject's "(#N)" suffix
	// is taken to be a pull request in repoName. See DefaultPRNumberCutoff.
	prNumberCutoff time.Time
	// debug reports whether to log how the changelog was derived.
	debug bool
}

func main() {
	var (
		cfg config

		prNumberCutoff = kingpin.Flag(
			"pr-number-cutoff",
			"The date (YYYY-MM-DD) from which a commit's \"(#N)\" suffix is taken to be a PR in this repo. Earlier numbers belong to the repo's previous numbering.",
		).Envar("PR_NUMBER_CUTOFF").Default(DefaultPRNumberCutoff).String()
	)

	kingpin.Flag(
		"base-branch",
		"The base release branch to generate the changelog for, e.g. branch/v18 or branch/v18.11.8.",
	).Envar("BASE_BRANCH").Required().StringVar(&cfg.baseBranch)

	kingpin.Flag(
		"base-tag",
		"The tag/version to generate the changelog from, of the form vXX.Y.Z, e.g. v15.1.1. A release branch also works.",
	).Envar("BASE_TAG").Required().StringVar(&cfg.baseTag)

	kingpin.Flag(
		"repo-name",
		"The name of the repo to generate the changelog for.",
	).Envar("REPO_NAME").Default("core").StringVar(&cfg.repoName)

	kingpin.Flag(
		"submodule",
		"Whether the e enterprise repo is a submodule of the core repo.",
	).Envar("SUBMODULE").Default("false").BoolVar(&cfg.submodule)

	kingpin.Flag(
		"pr-links",
		"Whether to include markdown links to GitHub pull requests in the generated changelog.",
	).Envar("PR_LINKS").Default("false").BoolVar(&cfg.prLinks)

	kingpin.Flag(
		"debug",
		"Print commit ranges, commits, PR numbers, and suppressed entries used to generate the changelog.",
	).Envar("DEBUG").Default("false").BoolVar(&cfg.debug)

	kingpin.Arg("dir", "The directory of the teleport repo.").Required().StringVar(&cfg.dir)

	kingpin.Parse()

	cutoff, err := time.Parse(time.DateOnly, *prNumberCutoff)
	if err != nil {
		log.Fatalf("invalid --pr-number-cutoff %q, want YYYY-MM-DD: %v", *prNumberCutoff, err)
	}
	cfg.prNumberCutoff = cutoff

	if err := run(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config) error {
	ossRepo := git.NewRepo(cfg.dir)

	gh, err := github.NewClientFromGHAuth(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	ossPRs, err := releasedPRs(ctx, gh, ossRepo, cfg, cfg.repoName, "OSS", cfg.baseTag, cfg.baseBranch)
	if err != nil {
		return trace.Wrap(err)
	}

	ossTmpl := tmplNoLinks
	if cfg.prLinks {
		ossTmpl = tmplLinks
	}
	ossGen := &generator{gh: gh, repo: cfg.repoName, tmpl: ossTmpl}

	ossCL, err := ossGen.generate(ctx, ossPRs)
	if err != nil {
		return trace.Wrap(err)
	}

	var entCL string
	if cfg.submodule {
		entCL, err = entCLFromSubmodule(ctx, gh, ossRepo, cfg)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	fmt.Println(ossCL)
	if entCL != "" {
		fmt.Println("Enterprise:")
		fmt.Println(entCL)
	}

	return nil
}

// releasedPRs returns the pull requests to put in the changelog for
// baseRef..headRef, with the work baseRef already shipped removed.
//
// The range alone over-reports whenever baseRef is not an ancestor of headRef,
// which is the case for release branches that chain off one another and take
// base branch work through squash merges and cherry-picks. label names the repo
// in debug output.
func releasedPRs(ctx context.Context, gh *github.Client, repo *git.Repo, cfg config, ghRepo, label, baseRef, headRef string) ([]int, error) {
	candidates, err := repo.CommitsInRange(baseRef+".."+headRef, true)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.debug {
		log.Printf("%s commit range: %s..%s", label, baseRef, headRef)
		for _, commit := range candidates {
			log.Printf("%s commit: %s %s", label, commit.SHA[:9], commit.Subject)
		}
	}

	released, err := buildReleasedSet(ctx, gh, repo, org, ghRepo, baseRef, headRef, cfg.prNumberCutoff)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	keep, dropped := released.suppressed(candidates)
	if cfg.debug {
		for _, suppression := range dropped {
			log.Printf("%s suppressed (%s): %s %s",
				label, suppression.Reason, suppression.Commit.SHA[:9], suppression.Commit.Subject)
		}
		log.Printf("%s commits: %d candidates, %d suppressed, %d kept",
			label, len(candidates), len(dropped), len(keep))
	}

	prs := prNumbersOf(keep)
	if cfg.debug {
		log.Printf("%s PRs: %v", label, prs)
	}
	return prs, nil
}

func entCLFromSubmodule(ctx context.Context, gh *github.Client, ossRepo *git.Repo, cfg config) (string, error) {
	entRepo := git.NewRepo(filepath.Join(cfg.dir, "e"))

	// The enterprise repo is a submodule of the OSS repo, so resolve the
	// enterprise refs from the SHAs the OSS refs point the submodule at.
	eBranchSHA, err := ossRepo.ObjectSHAAtPath(cfg.baseBranch, "e")
	if err != nil {
		return "", trace.Wrap(err)
	}
	eTagSHA, err := ossRepo.ObjectSHAAtPath(cfg.baseTag, "e")
	if err != nil {
		return "", trace.Wrap(err)
	}

	const entRepoName = "teleport.e"
	entPRs, err := releasedPRs(ctx, gh, entRepo, cfg, entRepoName, "Enterprise", eTagSHA, eBranchSHA)
	if err != nil {
		return "", trace.Wrap(err)
	}

	entGen := &generator{gh: gh, repo: entRepoName, tmpl: tmplNoLinks}

	entCL, err := entGen.generate(ctx, entPRs)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return entCL, nil
}
