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

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/libs/gitexec"
	"github.com/gravitational/shared-workflows/libs/github"
)

var (
	baseBranch = kingpin.Flag(
		"base-branch",
		"The base release branch to generate the changelog for.  It will be of the form branch/v*",
	).Envar("BASE_BRANCH").String()

	baseTag = kingpin.Flag(
		"base-tag",
		"The tag/version to generate the changelog from. It will be of the form vXX.Y.Z, e.g. v15.1.1",
	).Envar("BASE_TAG").String()

	dir = kingpin.Arg("dir", "directory of the teleport repo.").Required().String()
)

func main() {
	kingpin.Parse()

	if err := prereqCheck(); err != nil {
		log.Fatal(err)
	}

	ossRepo, _, err := initGit(*dir)
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
	timeLastRelease, timeLastEntRelease, timeLastEntMod, err := getTimestamps(*dir, filepath.Join(*dir, "e"), lastVersion)
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
	ossCL, err := ossCLGen.generateChangelog(branch, timeLastRelease, timeNow)
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

func prereqCheck() error {
	if err := gitexec.IsAvailable(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// initGit will initialize git repo for teleport OSS and Enterprise.
func initGit(repoRootDir string) (oss, ent *git.Repository, err error) {
	oss, err = git.PlainOpen(repoRootDir)
	if err != nil {
		return oss, ent, trace.Wrap(err)
	}

	ent, err = git.PlainOpen(filepath.Join(repoRootDir, "e"))
	if err != nil {
		return oss, ent, trace.Wrap(err)
	}
	return
}

// getBranch will return branch if parsed otherwise will attempt to find it
// Branch should be in the format "branch/v*"
func getBranch(branch string, repo *git.Repository) (string, error) {
	if branch != "" {
		return branch, nil
	}
	// symbolic-ref HEAD
	ref, err := repo.Reference(plumbing.HEAD, true)
	if err != nil {
		return "", trace.Wrap(err, "not on a branch")
	}

	if !ref.Name().IsBranch() {
		return "", trace.BadParameter("not on a branch: %s", ref)
	}
	branch = ref.Name().String()

	// if the branch is not in the branch/v* format then check it's root
	if !strings.HasPrefix(branch, "branch/v") {
		fbranch, err := getForkedBranch(*dir)
		if err != nil {
			return "", trace.Wrap(err, "could not determine a root branch")
		}
		branch = fbranch
	}

	return branch, nil
}

// getForkedBranch will attempt to find a root branch for the current one that is in the format branch/v*
func getForkedBranch(dir string) (string, error) {
	forkPointRef, err := gitexec.RunCmd(dir, "merge-base", "--fork-point", "HEAD")
	if err != nil {
		return "", trace.Wrap(err)
	}
	fbranch, err := gitexec.RunCmd(dir, "branch", "--list", "branch/v*", "--contains", forkPointRef, "--format", "%(refname:short)")
	if err != nil {
		return "", trace.Wrap(err)
	}
	if fbranch == "" { // stdout is empty indicating the search failed
		return "", trace.Errorf("could not find a valid root branch")
	}
	return fbranch, nil
}

func getLastVersion(baseTag, dir string) (string, error) {
	if baseTag != "" {
		return baseTag, nil
	}

	lastVersion, err := makePrintVersion(dir)
	if err != nil {
		return "", trace.Wrap(err)
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

func getTimestamps(dir string, entDir string, lastVersion string) (lastRelease, lastEnterpriseRelease, lastEnterpriseModify string, err error) {
	// get timestamp since last release
	since, err := gitexec.RunCmd(dir, "show", "-s", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd", lastVersion)
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get timestamp of last release")
	}
	// get timestamp of last enterprise release
	sinceEnt, err := gitexec.RunCmd(dir, "-C", "e", "show", "-s", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd", lastVersion)
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get timestamp of last enterprise release")
	}
	// get timestamp of last commit of enterprise
	entTime, err := gitexec.RunCmd(entDir, "log", "-n", "1", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd")
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get last modified time of e")
	}
	return since, sinceEnt, entTime, nil
}
