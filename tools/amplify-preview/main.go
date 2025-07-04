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
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/alecthomas/kingpin/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"
)

var (
	amplifyAppIDs  = kingpin.Flag("amplify-app-ids", "List of Amplify App IDs").Envar("AMPLIFY_APP_IDS").Required().Strings()
	gitBranchName  = kingpin.Flag("git-branch-name", "Git branch name").Envar("GIT_BRANCH_NAME").Required().String()
	createBranches = kingpin.Flag("create-branches",
		"Defines whether Amplify branches should be created if missing, or just lookup existing ones").Envar("CREATE_BRANCHES").Default("false").Bool()
	wait = kingpin.Flag("wait",
		"Wait for pending/running job to complete").Envar("WAIT").Default("false").Bool()
	waitRetries = kingpin.Flag("wait-retries",
		"Number of attempts to wait for pending/running job to complete").Envar("WAIT_RETRIES").Default("40").Int()
	waitInterval = kingpin.Flag("wait-interval",
		"Interval between attempts to wait for pending/running job to complete").Envar("WAIT_INTERVAL").Default("30s").Duration()
)

func main() {
	kingpin.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ctx, cancel := handleInterruption(context.Background())
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("run failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRetryer(func() aws.Retryer {
		return retry.AddWithMaxAttempts(retry.NewStandard(), 10)
	}))
	if err != nil {
		return fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	if amplifyAppIDs == nil || len(*amplifyAppIDs) == 0 {
		return errors.New("expected one more more Amplify App IDs")
	}

	amp := AmplifyPreview{
		client:     amplify.NewFromConfig(cfg),
		branchName: *gitBranchName,
		appIDs:     *amplifyAppIDs,
	}

	if len(amp.appIDs) == 1 {
		// kingpin env variables are separated by new lines, and there is no way to change the behavior
		// https://github.com/alecthomas/kingpin/issues/249
		amp.appIDs = strings.Split(amp.appIDs[0], ",")
	}

	branch, err := ensureAmplifyBranch(ctx, amp)
	if err != nil {
		return err
	}

	currentJob, activeJob, err := ensureAmplifyDeployment(ctx, amp, branch)
	if err != nil {
		return err
	}

	if err := postPreviewURL(ctx, amplifyJobsToMarkdown(branch, currentJob, activeJob)); err != nil {
		return fmt.Errorf("failed to post preview URL: %w", err)
	}

	setAmplifyInfoToGithubOutputs(branch, currentJob)

	slog.Info("Successfully posted PR comment")

	if *wait {
		currentJob, activeJob, err = amp.WaitForJobCompletion(ctx, branch, currentJob)
		if errors.Is(err, errJobTimeoutReached) {
			return fmt.Errorf("job did not complete within the specified timeout (%dx%s). Please retry this job again manually", *waitRetries, waitInterval.String())
		} else if err != nil {
			return fmt.Errorf("failed to follow job status: %w", err)
		}

		// update comment when job is completed
		if err := postPreviewURL(ctx, amplifyJobsToMarkdown(branch, currentJob, activeJob)); err != nil {
			return fmt.Errorf("failed to post preview URL: %w", err)
		}
	}

	if currentJob.Status == types.JobStatusFailed {
		slog.Error("amplify job is in failed state", logKeyBranchName, amp.branchName, "job_status", currentJob.Status, "job_id", *currentJob.JobId)
		return fmt.Errorf("amplify job is in %q state", currentJob.Status)
	}

	slog.Info("amplify job completed", logKeyBranchName, amp.branchName, "job_status", currentJob.Status, "job_id", *currentJob.JobId)

	return nil
}

// ensureAmplifyBranch checks if git branch is connected to amplify across multiple amplify apps
// if "create-branch" is enabled, then branch is created if not found, otherwise returns error
func ensureAmplifyBranch(ctx context.Context, amp AmplifyPreview) (*types.Branch, error) {
	branch, err := amp.FindExistingBranch(ctx)
	if err == nil {
		return branch, nil
	} else if errors.Is(err, errBranchNotFound) && *createBranches {
		return amp.CreateBranch(ctx)
	} else {
		return nil, fmt.Errorf("failed to lookup branch %q: %w", amp.branchName, err)
	}
}

// ensureAmplifyDeployment lists deployments and checks for latest and active (the one that's currently live) deployments
// if this branch has no deployments yet and "create-branch" is enabled, then deployment will be kicked off
// this is because when new branch is created on Amplify and "AutoBuild" is enabled, it won't start the deployment until next commit
func ensureAmplifyDeployment(ctx context.Context, amp AmplifyPreview, branch *types.Branch) (currentJob, activeJob *types.JobSummary, err error) {
	currentJob, activeJob, err = amp.GetLatestAndActiveJobs(ctx, branch)
	if err == nil {
		return currentJob, activeJob, nil
	} else if errors.Is(err, errNoJobForBranch) && *createBranches {
		currentJob, err = amp.StartJob(ctx, branch)
		return currentJob, activeJob, err
	} else {
		return nil, nil, fmt.Errorf("failed to lookup amplify job for branch %q: %w", amp.branchName, err)
	}
}

func setAmplifyInfoToGithubOutputs(branch *types.Branch, job *types.JobSummary) {
	kv := make(map[string]string)

	if branch.BranchName != nil {
		kv["AMPLIFY_BRANCH"] = *branch.BranchName
	}

	if branch.BranchArn != nil {
		if appId, err := appIDFromBranchARN(*branch.BranchArn); err != nil {
			slog.Error("failed to extract app ID from branch ARN", "branch_arn", *branch.BranchArn, "error", err)
		} else {
			kv["AMPLIFY_APP_ID"] = appId
		}
	}

	if job.JobId != nil {
		kv["AMPLIFY_JOB_ID"] = *job.JobId
	}

	if err := setGithubOutputs(kv); err != nil {
		slog.Error("failed to set Amplify info to GitHub outputs", "error", err)
	}
}

func handleInterruption(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		select {
		case <-c:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, func() {
		signal.Stop(c)
		cancel()
	}
}
