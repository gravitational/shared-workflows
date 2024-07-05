package git

import (
	"fmt"

	go_git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gravitational/trace"
)

// Repo provides git
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

func (r *Repo) GetCommitForTag(tag string) (*object.Commit, error) {
	t, err := r.Repository.Tag(tag)
	if err != nil {
		return &object.Commit{}, trace.Wrap(err, fmt.Sprintf("tag %q not found, use a valid git tag", tag))
	}
	return r.Repository.CommitObject(t.Hash())
}

func (r *Repo) GetCommitForHEAD() (*object.Commit, error) {
	ref, err := r.Repository.Reference(plumbing.HEAD, false)
	if err != nil {
		return &object.Commit{}, trace.Wrap(err, "can't get latest reference for ent")
	}
	return r.Repository.CommitObject(ref.Hash())
}

// GetCurrentBranch attempts to get the resolved branch of HEAD.
func (r *Repo) GetCurrentBranch() (string, error) {
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
