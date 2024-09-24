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

package git

import (
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// Repo provides a collection of functions to query and modify a single git repository.
// Wrapper around the go-git library that also includes methods to execute git commands
// on the system for missing compatability.
type Repo struct {
	dir    string
	runner commandRunner
}

const (
	// git should be expected to to output in strict ISO 8601 format
	gitTimeFormat = "2006-01-02T15:04:05Z07:00"
)

// NewRepoFromDirectory initializes [Repo] from a directory.
func NewRepoFromDirectory(dir string) (*Repo, error) {
	return &Repo{
		dir:    dir,
		runner: &defaultRunner{},
	}, nil
}

// BranchNameForHead will get the name of branch currently on.
// If not on a branch will return an error.
func (r *Repo) BranchNameForHead() (string, error) {
	// get ref
	ref, err := r.RunCmd("symbolic-ref", "HEAD")
	if err != nil {
		return "", trace.Wrap(err, "not on a branch")
	}

	// remove prefix and ensure that branch is in expected format
	branch, _ := strings.CutPrefix(ref, "refs/heads/")
	if branch == ref {
		return "", trace.BadParameter("not on a branch: %s", ref)
	}
	return branch, nil
}

// TimestampForRef will get the timestamp for the given reference.
// Will work for symbolic references such as tags, HEAD, branches
func (r *Repo) TimestampForRef(ref string) (time.Time, error) {
	t, err := r.RunCmd("show", "-s", "--format=%cI", ref)
	if err != nil {
		return time.Time{}, trace.Wrap(err, trace.BadParameter("can't get timestamp for ref: %q", ref))
	}
	return time.Parse(gitTimeFormat, t)
}

// TimestampForLatestCommit will get the timestamp for the last commit.
func (r *Repo) TimestampForLatestCommit() (time.Time, error) {
	t, err := r.RunCmd("log", "-n", "1", "--format=%cI")
	if err != nil {
		return time.Time{}, trace.Wrap(err, trace.BadParameter("can't get timestamp for latest commit on repo: %q", r.dir))
	}
	return time.Parse(gitTimeFormat, t)
}

// GetParentReleaseBranch will attempt to find a parent branch for HEAD.
// This will also work if HEAD has branched from an earlier commit in a release branch e.g.
//
//	 o---o---HEAD
//	/---o---o---branch/v16
func (r *Repo) GetParentReleaseBranch() (string, error) {
	forkPointRef, err := r.RunCmd("merge-base", "--fork-point", "HEAD")
	if err != nil {
		return "", trace.Wrap(err)
	}
	fbranch, err := r.RunCmd("branch", "--list", "branch/v*", "--contains", forkPointRef, "--format", "%(refname:short)")
	if err != nil {
		return "", trace.Wrap(err)
	}
	if fbranch == "" { // stdout is empty indicating the search failed
		return "", trace.Errorf("could not find a valid root branch")
	}
	return fbranch, nil
}
