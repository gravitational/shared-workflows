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
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/libs/git"
	"github.com/gravitational/shared-workflows/libs/github"
)

var (
	baseBranch = kingpin.Flag(
		"base-branch",
		"The base release branch to generate the changelog for. It will be of the form branch/v*",
	).Envar("BASE_BRANCH").String()

	baseTag = kingpin.Flag(
		"base-tag",
		"The tag/version to generate the changelog from. It will be of the form vXX.Y.Z, e.g. v15.1.1",
	).Envar("BASE_TAG").String()

	privateRepoName = kingpin.Flag(
		"private-repo-name",
		"The name of the private repository.",
	).Envar("PRIVATE_REPO_NAME").String()

	monoRepoEnabled = kingpin.Flag(
		"mono-repo-enabled",
		"Whether the repository is a mono-repo.",
	).Envar("MONO_REPO_ENABLED").Bool()

	dir = kingpin.Arg("dir", "directory of the teleport repo.").Required().String()
)

func main() {
	kingpin.Parse()

	ossRepo, err := git.NewRepoFromDirectory(*dir)
	if err != nil {
		log.Fatal(err)
	}

	// Figure out the branch and last version released for that branch
	branch, err := getBranch(*baseBranch, ossRepo)
	if err != nil {
		log.Fatal(err)
	}

	lastVersion, err := getLastVersion(*baseTag, *dir)
	if err != nil {
		log.Fatal(trace.Wrap(err, "failed to determine last version"))
	}

	// Determine timestamps of releases which is used to limit Github search
	timeLastRelease, err := ossRepo.TimestampForRef(lastVersion)
	if err != nil {
		log.Fatal(err)
	}

	cl, err := github.NewClientFromGHAuth(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// This is to exclude the commit of the last release from the changelog.
	timeLastRelease = timeLastRelease.Add(1 * time.Second)

	// By default, we pull from the OSS repository
	// However, we have situations where we need to pull from a private repository.
	// In these cases, the PR links are not relevant to external users so we exclude them.
	var repoName = "teleport"
	var excludePRLinks = false
	if *privateRepoName != "" {
		repoName = *privateRepoName
		excludePRLinks = true
	}

	// Generate changelogs
	ctx := context.Background()
	coreRepoScraper := &changelogScraper{
		ghclient: cl,
		repo:     repoName,
	}
	changelogs, err := coreRepoScraper.scrapeForChangelogs(ctx, branch, timeLastRelease, github.SearchTimeNow)
	if err != nil {
		log.Fatal(err)
	}

	if !*monoRepoEnabled { // If not a monorepo, we need to gather enterprise changelogs from the enterprise repository
		enterpriseChangelogs, err := gatherEnterpriseChangelogs(ctx, cl, branch, lastVersion)
		if err != nil {
			log.Fatal(err)
		}
		changelogs = append(changelogs, enterpriseChangelogs...)
	}

	rendered, err := renderChangelog(renderOpts{
		changelogs:         changelogs,
		excludeCorePRLinks: excludePRLinks,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(rendered)

}

// gatherEnterpriseChangelogs gathers the changelogs from the enterprise-only repository.
func gatherEnterpriseChangelogs(ctx context.Context, cl *github.Client, branch string, lastVersion string) ([]changelogInfo, error) {
	// Initialize from the e subdir which contains the repository information for enterprise-only changes
	enterpriseRepo, err := git.NewRepoFromDirectory(filepath.Join(*dir, "e"))
	if err != nil {
		return []changelogInfo{}, trace.Wrap(err)
	}

	timeLastEntRelease, timeLastEntMod, err := entTimestamps(enterpriseRepo, lastVersion)
	if err != nil {
		return []changelogInfo{}, trace.Wrap(err)
	}
	timeLastEntRelease = timeLastEntRelease.Add(1 * time.Second)
	timeLastEntMod = timeLastEntMod.Add(1 * time.Second)

	entCLGen := &changelogScraper{
		ghclient:          cl,
		repo:              "teleport.e",
		markAllEnterprise: true,
	}

	entCL, err := entCLGen.scrapeForChangelogs(ctx, branch, timeLastEntRelease, timeLastEntMod)
	if err != nil {
		return []changelogInfo{}, trace.Wrap(err)
	}
	return entCL, nil
}

// getBranch will return branch if parsed otherwise will attempt to find it
// Branch should be in the format "branch/v*"
func getBranch(branch string, repo *git.Repo) (string, error) {
	if branch != "" {
		return branch, nil
	}

	branch, err := repo.BranchNameForHead()
	if err != nil {
		return "", trace.Wrap(err)
	}

	// if the branch is not in the branch/v* format then check it's root
	if !strings.HasPrefix(branch, "branch/v") {
		fbranch, err := repo.GetParentReleaseBranch()
		if err != nil {
			return "", trace.Wrap(err, "could not determine a root branch")
		}
		branch = fbranch
	}

	return branch, nil
}

func getLastVersion(baseTag, dir string) (string, error) {
	if baseTag != "" {
		return baseTag, nil
	}

	lastVersion, err := makePrintVersion(dir)
	if err != nil {
		return "", trace.Wrap(err, "base-tag was not set, defaulted to invoking 'make version' but failed")
	}

	return lastVersion, nil
}

// makePrintVersion will run 'make -s print-version'
func makePrintVersion(dir string) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command("make", "-s", "print-version")
	cmd.Dir = dir
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	err := cmd.Run()

	if err != nil {
		return strings.TrimSpace(stderr.String()), trace.Wrap(err, "can't get last released version")
	}
	out := strings.TrimSpace(stdout.String())

	return "v" + out, nil
}

func entTimestamps(entRepo *git.Repo, tag string) (lastRelease, lastCommit time.Time, err error) {
	// get timestamp since last release
	lastRelease, err = entRepo.TimestampForRef(tag)
	if err != nil {
		return lastRelease, lastCommit, trace.Wrap(err, "can't get commit for tag")
	}

	// get timestamp of last commit
	lastCommit, err = entRepo.TimestampForLatestCommit()
	if err != nil {
		return lastRelease, lastCommit, trace.Wrap(err, "can't get timestamp for ent")
	}

	return lastRelease, lastCommit, nil
}
