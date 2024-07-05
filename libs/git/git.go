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
	"fmt"

	go_git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gravitational/trace"
)

// Repo provides utility functions around a git repository.
// It wraps the go-git library and also provides a method for executing
// git commands on the same repository.
type Repo struct {
	dir        string
	Repository *go_git.Repository
}

func NewRepoFromDirectory(dir string) (*Repo, error) {
	inner, err := go_git.PlainOpen(dir)
	if err != nil {
		return &Repo{}, trace.Wrap(err)
	}

	return &Repo{
		dir:        dir,
		Repository: inner,
	}, nil
}

// GetCommitForTag attempts to get the resolved commit for a given tag.
func (r *Repo) GetCommitForTag(tag string) (*object.Commit, error) {
	t, err := r.Repository.Tag(tag)
	if err != nil {
		return &object.Commit{}, trace.Wrap(err, fmt.Sprintf("tag %q not found, use a valid git tag", tag))
	}
	return r.Repository.CommitObject(t.Hash())
}

// GetCommitForHead attempts to get the resolved commit for head.
func (r *Repo) GetCommitForHead() (*object.Commit, error) {
	ref, err := r.Repository.Reference(plumbing.HEAD, false)
	if err != nil {
		return &object.Commit{}, trace.Wrap(err, "can't get latest reference for ent")
	}
	return r.Repository.CommitObject(ref.Hash())
}

// GetBranchForHead attempts to get the resolved branch of HEAD.
func (r *Repo) GetBranchNameForHead() (string, error) {
	// symbolic-ref HEAD
	ref, err := r.Repository.Reference(plumbing.HEAD, true)
	if err != nil {
		return "", trace.Wrap(err, "not on a branch")
	}

	if !ref.Name().IsBranch() {
		return "", trace.BadParameter("not on a branch: %s", ref)
	}
	return ref.Name().String(), nil
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
