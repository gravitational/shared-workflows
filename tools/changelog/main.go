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

	dir = kingpin.Arg("dir", "directory of the teleport repo.").Required().String()
)

func main() {
	kingpin.Parse()

	ossRepo, entRepo, err := initGit(*dir)
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
	timeLastRelease, err := ossTimestamp(ossRepo, lastVersion)
	if err != nil {
		log.Fatal(err)
	}
	timeLastEntRelease, timeLastEntMod, err := entTimestamps(entRepo, lastVersion)
	if err != nil {
		log.Fatal(err)
	}

	cl, err := github.NewClientFromGHAuth(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// Generate changelogs
	ossCLGen := &changelogGenerator{
		isEnt:    false,
		ghclient: cl,
		repo:     "teleport",
	}
	entCLGen := &changelogGenerator{
		isEnt:    true,
		ghclient: cl,
		repo:     "teleport.e",
	}
	ossCL, err := ossCLGen.generateChangelog(branch, timeLastRelease, github.SearchTimeNow)
	if err != nil {
		log.Fatal(err)
	}
	entCL, err := entCLGen.generateChangelog(branch, timeLastEntRelease, timeLastEntMod)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(ossCL)
	if entCL != "" {
		fmt.Println("Enterprise:")
		fmt.Println(entCL)
	}
}

// initGit will initialize git repo for teleport OSS and Enterprise.
func initGit(repoRootDir string) (oss, ent *git.Repo, err error) {
	oss, err = git.NewRepoFromDirectory(repoRootDir)
	if err != nil {
		return oss, ent, trace.Wrap(err)
	}

	ent, err = git.NewRepoFromDirectory(filepath.Join(repoRootDir, "e"))
	if err != nil {
		return oss, ent, trace.Wrap(err)
	}
	return
}

// getBranch will return branch if parsed otherwise will attempt to find it
// Branch should be in the format "branch/v*"
func getBranch(branch string, repo *git.Repo) (string, error) {
	if branch != "" {
		return branch, nil
	}

	branch, err := repo.GetBranchNameForHead()
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
		return "", trace.Wrap(err, "base-tag was not provided, make does not have 'version' target")
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

func ossTimestamp(ossRepo *git.Repo, tag string) (time.Time, error) {
	var timestamp time.Time
	// get timestamp since last release
	versionCommit, err := ossRepo.GetCommitForTag(tag)
	if err != nil {
		return timestamp, err
	}
	timestamp = versionCommit.Author.When
	return timestamp, nil
}

func entTimestamps(entRepo *git.Repo, tag string) (lastRelease, lastCommit time.Time, err error) {
	// get timestamp since last release
	versionCommit, err := entRepo.GetCommitForTag(tag)
	if err != nil {
		return lastRelease, lastCommit, trace.Wrap(err, "can't get commit for tag")
	}
	lastRelease = versionCommit.Author.When

	// get timestamp of last commit
	comm, err := entRepo.GetCommitForHead()
	if err != nil {
		return lastRelease, lastCommit, trace.Wrap(err, "can't get timestamp for ent")
	}
	lastCommit = comm.Author.When

	return lastRelease, lastCommit, nil
}
