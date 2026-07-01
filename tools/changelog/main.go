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

		dir = kingpin.Arg("dir", "The directory of the teleport repo.").Required().String()
	)
	kingpin.Parse()

	if err := run(context.Background(), *dir, *baseBranch, *baseTag); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, dir, baseBranch, baseTag string) error {
	ossRepo := git.NewRepo(dir)
	entRepo := git.NewRepo(filepath.Join(dir, "e"))

	// The enterprise repo is a submodule of the OSS repo, so resolve the
	// enterprise refs from the SHAs the OSS refs point the submodule at.
	eBranchSHA, err := ossRepo.ObjectSHAAtPath(baseBranch, "e")
	if err != nil {
		return trace.Wrap(err)
	}
	eTagSHA, err := ossRepo.ObjectSHAAtPath(baseTag, "e")
	if err != nil {
		return trace.Wrap(err)
	}

	ossPRs, err := ossRepo.PRsBetweenRefs(baseTag, baseBranch)
	if err != nil {
		return trace.Wrap(err)
	}
	entPRs, err := entRepo.PRsBetweenRefs(eTagSHA, eBranchSHA)
	if err != nil {
		return trace.Wrap(err)
	}

	gh, err := github.NewClientFromGHAuth(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	ossGen := &generator{gh: gh, repo: "teleport", tmpl: tmplLinks}
	entGen := &generator{gh: gh, repo: "teleport.e", tmpl: tmplNoLinks}

	ossCL, err := ossGen.generate(ctx, ossPRs)
	if err != nil {
		return trace.Wrap(err)
	}
	entCL, err := entGen.generate(ctx, entPRs)
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Println(ossCL)
	if entCL != "" {
		fmt.Println("Enterprise:")
		fmt.Println(entCL)
	}

	return nil
}
