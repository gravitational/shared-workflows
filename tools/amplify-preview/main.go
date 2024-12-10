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
	"errors"
	"log/slog"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"
)

var (
	logger = slog.New(slog.NewTextHandler(os.Stderr, nil))

	amplifyAppIDs  = kingpin.Flag("amplify-app-ids", "List of Amplify App IDs").Envar("AMPLIFY_APP_IDS").Required().Strings()
	gitBranchName  = kingpin.Flag("git-branch-name", "Git branch name").Envar("GIT_BRANCH_NAME").Required().String()
	createBranches = kingpin.Flag("crate-branches",
		"Defines whether Amplify branches should be created if missing, or just lookup existing ones").Envar("CREATE_BRANCHES").Default("false").Bool()
)

func main() {

	kingpin.Parse()
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRetryer(func() aws.Retryer {
		return retry.AddWithMaxAttempts(retry.NewStandard(), 10)
	}))
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	amp := AmplifyPreview{
		client: amplify.NewFromConfig(cfg),
		appIDs: *amplifyAppIDs,
	}

	// Check if Amplify branch is already connected to one of the Amplify Apps
	branch, err := amp.FindExistingBranch(ctx, *gitBranchName)
	if err != nil {
		logger.Error("failed to lookup branch", logKeyBranchName, *gitBranchName, "error", err)
		os.Exit(1)
	}

	// If branch wasn't found, and branch creation enabled - create new branch
	if branch == nil && *createBranches {
		branch, err = amp.CreateBranch(ctx, *gitBranchName)
		if err != nil {
			logger.Error("failed to create branch", logKeyBranchName, *gitBranchName, "error", err)
			os.Exit(1)
		}
	}

	// check if existing branch was/being already deployed
	job, err := amp.GetJob(ctx, branch, nil)
	if err != nil {
		logger.Error("failed to get amplify job", logKeyBranchName, *gitBranchName, "error", err)
		os.Exit(1)
	}

	if errors.Is(err, errNoJobForBranch) && *createBranches {
		job, err = amp.StartJob(ctx, branch)
		if err != nil {
			logger.Error("failed to start amplify job", logKeyBranchName, *gitBranchName, "error", err)
			os.Exit(1)
		}
	}

	if err := postPreviewURL(ctx, amplifyJobToMarkdown(job, branch)); err != nil {
		logger.Error("failed to post preview URL", "error", err)
		os.Exit(1)
	}

	logger.Error("amplify job is in failed state", "job_status", job.Status, "job_id", job.JobId)
	if job.Status == types.JobStatusFailed {
		os.Exit(1)
	}
}
