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

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/libs/git"
	"github.com/gravitational/shared-workflows/libs/github"
)

func main() {
	var (
		baseBranch = kingpin.Flag(
			"base-branch",
			"The base release branch to generate the changelog for, of the form branch/v*.",
		).Envar("BASE_BRANCH").Required().String()

		baseTag = kingpin.Flag(
			"base-tag",
			"The tag/version to generate the changelog from, of the form vXX.Y.Z, e.g. v15.1.1.",
		).Envar("BASE_TAG").Required().String()

		repoName = kingpin.Flag(
			"repo-name",
			"The name of the repo to generate the changelog for.",
		).Envar("REPO_NAME").Default("core").String()

		submodule = kingpin.Flag(
			"submodule",
			"Whether the e enterprise repo is a submodule of the core repo.",
		).Envar("SUBMODULE").Default("false").Bool()

		prLinks = kingpin.Flag(
			"pr-links",
			"Whether to include markdown links to GitHub pull requests in the generated changelog.",
		).Envar("PR_LINKS").Default("false").Bool()

		debug = kingpin.Flag(
			"debug",
			"Print commit ranges, commits, and PR numbers used to generate the changelog.",
		).Envar("DEBUG").Default("false").Bool()

		dir = kingpin.Arg("dir", "The directory of the teleport repo.").Required().String()
	)
	kingpin.Parse()

	if err := run(context.Background(), *repoName, *dir, *baseBranch, *baseTag, *submodule, *prLinks, *debug); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, repoName, dir, baseBranch, baseTag string, submodule, prLinks, debug bool) error {
	ossRepo := git.NewRepo(dir)

	if debug {
		log.Printf("OSS commit range: %s..%s", baseTag, baseBranch)
		commits, err := ossRepo.CommitLinesBetweenRefs(baseTag, baseBranch)
		if err != nil {
			return trace.Wrap(err)
		}
		for _, commit := range commits {
			log.Printf("OSS commit: %s", commit)
		}
	}

	ossPRs, err := ossRepo.PRsBetweenRefs(baseTag, baseBranch)
	if err != nil {
		return trace.Wrap(err)
	}
	if debug {
		log.Printf("OSS PRs: %v", ossPRs)
	}

	gh, err := github.NewClientFromGHAuth(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	ossTmpl := tmplNoLinks
	if prLinks {
		ossTmpl = tmplLinks
	}
	ossGen := &generator{gh: gh, repo: repoName, tmpl: ossTmpl}

	ossCL, err := ossGen.generate(ctx, ossPRs)
	if err != nil {
		return trace.Wrap(err)
	}

	var entCL string
	if submodule {
		entCL, err = entCLFromSubmodule(ctx, gh, ossRepo, dir, baseBranch, baseTag, debug)
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

func entCLFromSubmodule(ctx context.Context, gh *github.Client, ossRepo *git.Repo, dir, baseBranch, baseTag string, debug bool) (string, error) {
	entRepo := git.NewRepo(filepath.Join(dir, "e"))

	// The enterprise repo is a submodule of the OSS repo, so resolve the
	// enterprise refs from the SHAs the OSS refs point the submodule at.
	eBranchSHA, err := ossRepo.ObjectSHAAtPath(baseBranch, "e")
	if err != nil {
		return "", trace.Wrap(err)
	}
	eTagSHA, err := ossRepo.ObjectSHAAtPath(baseTag, "e")
	if err != nil {
		return "", trace.Wrap(err)
	}
	entPRs, err := entRepo.PRsBetweenRefs(eTagSHA, eBranchSHA)
	if err != nil {
		return "", trace.Wrap(err)
	}
	if debug {
		log.Printf("Enterprise commit range: %s..%s", eTagSHA, eBranchSHA)
		commits, err := entRepo.CommitLinesBetweenRefs(eTagSHA, eBranchSHA)
		if err != nil {
			return "", trace.Wrap(err)
		}
		for _, commit := range commits {
			log.Printf("Enterprise commit: %s", commit)
		}
		log.Printf("Enterprise PRs: %v", entPRs)
	}
	entGen := &generator{gh: gh, repo: "teleport.e", tmpl: tmplNoLinks}

	entCL, err := entGen.generate(ctx, entPRs)
	if err != nil {
		return "", trace.Wrap(err)
	}

	return entCL, nil
}
