/*
Copyright 2021 Gravitational, Inc.

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

package env

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
)

const (
	// Repo slugs
	AccessGraphRepo = "access-graph"
	CloudRepo       = "cloud"
	TeleportRepo    = "teleport"
	TeleportERepo   = "teleport.e"

	// Teams
	CoreTeam     = "Core"
	CloudTeam    = "Cloud"
	InternalTeam = "Internal"

	// Cloud Deploy Branches
	CloudProdBranch    = "prod"
	CloudStagingBranch = "staging"
)

// Environment is the execution environment the workflow is running in.
type Environment struct {
	// Organization is the GitHub organization (gravitational).
	Organization string

	// Repository is the GitHub repository (teleport).
	Repository string

	// Number is the PR number.
	Number int

	// RunID is the GitHub Actions workflow run ID.
	RunID int64

	// Author is the author of the PR.
	Author string

	// Additions is the number of new lines added in the PR
	Additions int

	// Deletions is the number of lines removed in the PR
	Deletions int

	// UnsafeHead is the name of the branch the workflow is running in.
	//
	// UnsafeHead can be attacker controlled and should not be used in any
	// security sensitive context. For example, don't use it when crafting a URL
	// to send a request to or an access decision. See the following link for
	// more details:
	//
	// https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#understanding-the-risk-of-script-injections
	UnsafeHead string

	// UnsafeBase is the name of the base branch the user is trying to merge the
	// PR into. For example: "master" or "branch/v8".
	//
	// UnsafeBase can be attacker controlled and should not be used in any
	// security sensitive context. For example, don't use it when crafting a URL
	// to send a request to or an access decision. See the following link for
	// more details:
	//
	// https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#understanding-the-risk-of-script-injections
	UnsafeBase string
}

// New returns a new execution environment for the workflow.
func New() (*Environment, error) {
	event, err := readEvent()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// If the event does not have a action associated with it (for example a cron
	// run), read in organization/repository from the environment.
	if event.Action == "" {
		organization, repository, err := readEnvironment()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return &Environment{
			Organization: organization,
			Repository:   repository,
		}, nil
	}

	runID, err := strconv.ParseInt(os.Getenv(githubRunID), 10, 64)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return &Environment{
		Organization: event.Repository.Owner.Login,
		Repository:   event.Repository.Name,
		Number:       event.PullRequest.Number,
		RunID:        runID,
		Author:       event.PullRequest.User.Login,
		Additions:    event.PullRequest.Additions,
		Deletions:    event.PullRequest.Deletions,
		UnsafeHead:   event.PullRequest.UnsafeHead.UnsafeRef,
		UnsafeBase:   event.PullRequest.UnsafeBase.UnsafeRef,
	}, nil
}

// IsCloudDeployBranch returns true when the environment's repository is cloud
// and the base branch is a deploy branch (e.g. staging or prod).
func (e *Environment) IsCloudDeployBranch() bool {
	return e.Repository == CloudRepo &&
		(e.UnsafeBase == CloudProdBranch || e.UnsafeBase == CloudStagingBranch)
}

// Team returns CloudTeam when the repository is the cloud repo otherwise CoreTeam.
func (e *Environment) Team() string {
	if e.Repository == CloudRepo {
		return CloudTeam
	}
	return CoreTeam
}

func readEvent() (*Event, error) {
	f, err := os.Open(os.Getenv(githubEventPath))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer f.Close()

	var event Event
	if err := json.NewDecoder(f).Decode(&event); err != nil {
		return nil, trace.Wrap(err)
	}

	return &event, nil
}

func readEnvironment() (string, string, error) {
	repository := os.Getenv(githubRepository)
	if repository == "" {
		return "", "", trace.BadParameter("%v environment variable missing", githubRepository)
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return "", "", trace.BadParameter("failed to parse organization and/or repository")
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", trace.BadParameter("invalid organization and/or repository")
	}
	return parts[0], parts[1], nil
}

const (
	// githubEventPath is an environment variable that contains a path to the
	// GitHub event for a workflow run.
	githubEventPath = "GITHUB_EVENT_PATH"

	// githubRepository is an environment variable that contains the organization
	// and repository name.
	githubRepository = "GITHUB_REPOSITORY"

	// githubRunID is an environment variable that contains the workflow run ID.
	githubRunID = "GITHUB_RUN_ID"
)
