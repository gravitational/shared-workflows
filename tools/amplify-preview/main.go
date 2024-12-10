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
	"log"

	"github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
)

var (
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
		log.Fatalln("failed to load configuration", err)
	}

	amp := AmplifyPreview{
		client: amplify.NewFromConfig(cfg),
		appIDs: *amplifyAppIDs,
	}

	// Check if Amplify branch is already connected to one of the Amplify Apps
	branch, err := amp.FindExistingBranch(ctx, *gitBranchName)
	if err != nil {
		log.Fatalf("Failed to lookup branch %q: %s", *gitBranchName, err)
	}

	// If branch wasn't found, and branch creation enabled - create new branch
	if branch == nil && *createBranches {
		branch, err = amp.CreateBranch(ctx, *gitBranchName)
		if err != nil {
			log.Fatalf("Failed to lookup create %q: %s", *gitBranchName, err)
		}
	}

	// check if existing branch was/being already deployed
	job, err := amp.GetJob(ctx, branch, nil)
	if err != nil {
		log.Fatalf("Failed to get amplify job %q: %s", *gitBranchName, err)
	}

	if errors.Is(err, errNoJobForBranch) && *createBranches {
		job, err = amp.StartJob(ctx, branch)
		if err != nil {
			log.Fatalf("Failed to start amplify job %q: %s", *gitBranchName, err)
		}
	}

	if err := postPreviewURL(ctx, amplifyJobToMarkdown(job, branch)); err != nil {
		log.Fatalln("Failed to post preview URL", err)
	}

}
