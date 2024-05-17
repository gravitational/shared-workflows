/*
 * Copyright 2024 Gravitational, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v62/github"
	checker "github.com/gravitational/shared-workflows/tools/pr-vuln-check/codecheckers"
	gochecker "github.com/gravitational/shared-workflows/tools/pr-vuln-check/codecheckers/go"
	"github.com/gravitational/trace"
)

var (
	Version = "0.0.0-dev"
)

func main() {
	kingpin.Version(Version)

	currentWorkingDirectory, _ := os.Getwd()
	ghToken := kingpin.Flag("github-token", "Token used to authenticate GitHub when making API calls").Envar("GITHUB_TOKEN").Required().String()
	owner := kingpin.Flag("owner", "The owner of the repository containing the PR to check").Envar("GITHUB_REPOSITORY_OWNER").Required().String()
	repo := kingpin.Flag("repo", "The name of the repository containing the PR to check").Envar("GITHUB_REPOSITORY").Required().String()
	pr := kingpin.Flag("pr-number", "The number of the PR to check").Required().Int()
	clonePath := kingpin.Flag("clone-path", "The path the the repo should be cloned to").Required().Default(currentWorkingDirectory).ExistingDir()
	repoToClone := kingpin.Flag("clone-repo", "The full name of the repo that should be cloned, defaulting to owner/repo").String()
	cloneToken := kingpin.Flag("clone-token", "Token used to authenticate GitHub when cloning the repo, defaulting to github-token").Required().String()

	goForceRunPaths := kingpin.Flag("go-force-run-paths", "A map of file paths that should always trigger a Go check, and the module that they should check").StringMap()

	kingpin.Parse()

	ctx := context.Background()
	err := NewApplicationInstance(*ghToken, *clonePath, *repoToClone, *cloneToken, *owner, *repo, *pr, *goForceRunPaths).Run(ctx)
	if err != nil {
		log.Fatalf("%v", err)
	}
}

type Application struct {
	gh           *github.Client
	owner        string
	repo         string
	pr           int
	clonePath    string
	repoToClone  string
	cloneToken   string
	codeCheckers []checker.CodeChecker
}

func NewApplicationInstance(ghToken, clonePath, repoToClone, cloneToken, owner, repo string, pr int, goForceRunPaths map[string]string) *Application {
	ghClient := github.NewClient(nil)
	if ghToken != "" {
		ghClient = ghClient.WithAuthToken(ghToken)
	}

	// Trim the owner in case an owner/repo value is provided instead of just owner.
	repo = strings.TrimLeft(repo, owner+"/")

	if repoToClone == "" {
		repoToClone = fmt.Sprintf("%s/%s", owner, repo)
	}

	if cloneToken == "" {
		cloneToken = ghToken
	}

	return &Application{
		gh:          ghClient,
		owner:       owner,
		repo:        repo,
		pr:          pr,
		clonePath:   clonePath,
		repoToClone: repoToClone,
		cloneToken:  cloneToken,
		codeCheckers: []checker.CodeChecker{
			gochecker.NewGoChecker(goForceRunPaths),
		},
	}
}

func (a *Application) Run(ctx context.Context) error {
	changedFilePaths, err := a.getChangedFilePaths(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to get a list of changed file paths")
	}

	shouldContinue := a.shouldProcessVulnerabilities(changedFilePaths)
	if !shouldContinue {
		return nil
	}

	err = a.downloadRepo(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to download repo")
	}

	return a.processVulnerabilities(ctx, changedFilePaths)
}

func (a *Application) shouldProcessVulnerabilities(changedFilePaths []string) bool {
	for _, checker := range a.codeCheckers {
		if checker.ShouldCheckForVulnerabilities(changedFilePaths) {
			return true
		}
	}

	return false
}

func (a *Application) getChangedFilePaths(ctx context.Context) ([]string, error) {
	prFiles, err := a.getPrFiles(ctx)
	if err != nil {
		return nil, trace.Wrap(err, "failed to get all PR files")
	}

	changedFiles := make([]string, 0, len(prFiles))
	for _, prFile := range prFiles {
		status := prFile.GetStatus()
		if slices.Contains([]string{"added", "modified", "changed"}, status) {
			changedFiles = append(changedFiles, *prFile.Filename)
		}
	}

	return changedFiles, nil
}

func (a *Application) getPrFiles(ctx context.Context) ([]*github.CommitFile, error) {
	opt := &github.ListOptions{
		PerPage: 3000, // This is the maximum according to the docs
	}

	// Get all pages of results
	var allPrFiles []*github.CommitFile
	for {
		prFiles, resp, err := a.gh.PullRequests.ListFiles(ctx, a.owner, a.repo, a.pr, opt)
		if err != nil {
			return nil, trace.Wrap(err, "failed to pull all PR files for results page %d", opt.Page)
		}

		allPrFiles = append(allPrFiles, prFiles...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allPrFiles, nil
}

func (a *Application) downloadRepo(ctx context.Context) error {
	repoUrl := "https://github.com/" + a.repoToClone
	var auth transport.AuthMethod
	if a.cloneToken != "" {
		auth = &http.BasicAuth{
			Username: "x-access-token",
			Password: a.cloneToken,
		}
	}

	_, err := git.PlainCloneContext(ctx, a.clonePath, false, &git.CloneOptions{
		Auth:              auth,
		URL:               repoUrl,
		Progress:          os.Stdout,
		RecurseSubmodules: git.DefaultSubmoduleRecursionDepth,
		Depth:             1,
	})

	if err != nil {
		return trace.Wrap(err, "failed to clone repo %q to %q", repoUrl, a.clonePath)
	}

	return nil
}

func (a *Application) processVulnerabilities(ctx context.Context, prChangedFilePaths []string) error {
	localChangedFilePaths := make([]string, 0, len(prChangedFilePaths))
	for _, prChangedFilePath := range prChangedFilePaths {
		localChangedFilePaths = append(localChangedFilePaths, filepath.Join(a.clonePath, prChangedFilePath))
	}

	errors := make([]error, 0, len(a.codeCheckers))
	for _, checker := range a.codeCheckers {
		err := checker.DoCheck(ctx, localChangedFilePaths)
		if err != nil {
			errors = append(errors, err)
		}
	}

	// Will return nil if all aggregated errors are nil
	return trace.NewAggregate(errors...)
}
