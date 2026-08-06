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
	"encoding/json"
	"os"

	"github.com/gravitational/shared-workflows/libs/github"
)

// fakeGitHubClient is a fake implementation of the GitHub client for testing purposes.
type fakeGitHubClient struct {
	testFileName string
}

func (f *fakeGitHubClient) ListChangelogPullRequests(ctx context.Context, owner, repo string, opts *github.ListChangelogPullRequestsOpts) ([]github.ChangelogPR, error) {
	testFile, err := os.Open(f.testFileName)
	if err != nil {
		return []github.ChangelogPR{}, err
	}
	defer testFile.Close()

	decoder := json.NewDecoder(testFile)
	var changelogPRs []github.ChangelogPR
	if err := decoder.Decode(&changelogPRs); err != nil {
		return []github.ChangelogPR{}, err
	}

	// For testing purposes, we can return a hardcoded response or an error
	return changelogPRs, nil
}
