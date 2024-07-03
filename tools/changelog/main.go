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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/libs/git"
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
)

func main() {
	kingpin.Parse()

	if err := prereqCheck(); err != nil {
		log.Fatal(err)
	}

	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(trace.Wrap(err, "failed to get working directory"))
	}

	topDir, err := git.RunCmd(workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		log.Fatal(err)
	}
	entDir := filepath.Join(topDir, "e")

	// Figure out the branch and last version released for that branch
	branch, err := getBranch(*baseBranch, workDir)
	if err != nil {
		log.Fatal(err)
	}

	lastVersion, err := getLastVersion(*baseTag, workDir)
	if err != nil {
		log.Fatal(trace.Wrap(err, "failed to determine last version"))
	}

	// Determine timestamps of releases which is used to limit Github search
	timeLastRelease, timeLastEntRelease, timeLastEntMod, err := getTimestamps(topDir, entDir, lastVersion)
	if err != nil {
		log.Fatal(err)
	}

	// Generate changelogs
	ossCLGen := &changelogGenerator{
		isEnt: false,
		dir:   topDir,
	}
	entCLGen := &changelogGenerator{
		isEnt: true,
		dir:   entDir,
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
	if err := git.IsAvailable(); err != nil {
		return trace.Wrap(err)
	}
	if err := gh.IsAvailable(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// getBranch will return branch if parsed otherwise will attempt to find it
// Branch should be in the format "branch/v*"
func getBranch(branch, dir string) (string, error) {
	if branch != "" {
		return branch, nil
	}
	// get ref
	ref, err := git.RunCmd(dir, "symbolic-ref", "HEAD")
	if err != nil {
		return "", trace.Wrap(err, "not on a branch")
	}

	// remove prefix and ensure that branch is in expected format
	branch, _ = strings.CutPrefix(ref, "refs/heads/")
	if branch == ref {
		return "", trace.BadParameter("not on a branch: %s", ref)
	}

	// if the branch is not in the branch/v* format then check it's root
	if !strings.HasPrefix(branch, "branch/v") {
		fbranch, err := getForkedBranch(dir)
		if err != nil {
			return "", trace.Wrap(err, "could not determine a root branch")
		}
		branch = fbranch
	}

	return branch, nil
}

// getForkedBranch will attempt to find a root branch for the current one that is in the format branch/v*
func getForkedBranch(dir string) (string, error) {
	forkPointRef, err := git.RunCmd(dir, "merge-base", "--fork-point", "HEAD")
	if err != nil {
		return "", trace.Wrap(err)
	}
	fbranch, err := git.RunCmd(dir, "branch", "--list", "branch/v*", "--contains", forkPointRef, "--format", "%(refname:short)")
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

	// get root dir of repo
	topDir, err := git.RunCmd(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", trace.Wrap(err)
	}
	lastVersion, err := makePrintVersion(topDir)
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
	since, err := git.RunCmd(dir, "show", "-s", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd", lastVersion)
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get timestamp of last release")
	}
	// get timestamp of last enterprise release
	sinceEnt, err := git.RunCmd(dir, "-C", "e", "show", "-s", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd", lastVersion)
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get timestamp of last enterprise release")
	}
	// get timestamp of last commit of enterprise
	entTime, err := git.RunCmd(entDir, "log", "-n", "1", "--date=format:%Y-%m-%dT%H:%M:%S%z", "--format=%cd")
	if err != nil {
		return "", "", "", trace.Wrap(err, "can't get last modified time of e")
	}
	return since, sinceEnt, entTime, nil
}
