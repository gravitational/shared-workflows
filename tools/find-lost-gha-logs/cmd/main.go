/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	ghiter "github.com/enrichman/gh-iter"
	"github.com/google/go-github/v63/github"
)

type runTracker struct {
	run             *github.WorkflowRun
	jobsMissingLogs []*github.WorkflowJob
	jobsChecked     int
}

type workflowTracker struct {
	workflow           *github.Workflow
	runs               []*runTracker
	runsChecked        int
	hasJobsMissingLogs bool
}

type repoTracker struct {
	org       string
	repo      string
	workflows []*workflowTracker
}

type githubOpts struct {
	daysToCheck       int
	includeSelfHosted bool
}

func main() {
	org, repo, githubToken, opts, err := getFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	if err := run(org, repo, githubToken, opts); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func getFlags() (org, repo, githubToken string, opts githubOpts, err error) {
	flag.CommandLine.SetOutput(os.Stderr)

	flag.StringVar(&org, "org", "gravitational", "GitHub organization or owner to check against")
	flag.StringVar(&repo, "repo", "teleport.e", "GitHub repo to check against")
	flag.StringVar(&githubToken, "token", "${GITHUB_TOKEN}", "GitHub token to use (will default to ${GITHUB_TOKEN} env var if unset)")
	flag.IntVar(&opts.daysToCheck, "days-to-check", 90, "Including the current date, the number of days to look through for workflow jobs")
	flag.BoolVar(&opts.includeSelfHosted, "include-self-hosted", false, "True to include workflow runs without logs for self-hosted runner jobs, false otherwise")

	flag.Parse()

	if githubToken == "" {
		// Don't use this as the flag default value to prevent logging it to stdout when `--help` is provided
		githubToken = os.Getenv("GITHUB_TOKEN")

		if githubToken == "" {
			return org, repo, githubToken, opts, errors.New("github token not provided and GITHUB_TOKEN is unset")
		}
	}

	return
}

func run(org, repo, token string, opts githubOpts) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	client := github.NewClient(nil).WithAuthToken(token)

	repoTracker, err := processRepo(ctx, client, org, repo, opts)
	if err != nil {
		// Track the error but still log the output
		err = fmt.Errorf("failed to process repo %s/%s: %w", org, repo, err)
	}

	fmt.Print(getRepoResults(repoTracker, org, repo, opts.daysToCheck))

	return err
}

func getRepoResults(tracker *repoTracker, org, repo string, days int) string {
	// Aggregate metrics
	workflowsChecked, runsChecked, jobsChecked, jobsMissingLogs := len(tracker.workflows), 0, 0, 0
	for _, workflow := range tracker.workflows {
		runsChecked += workflow.runsChecked
		for _, run := range workflow.runs {
			jobsChecked += run.jobsChecked
			jobsMissingLogs += len(run.jobsMissingLogs)
		}
	}

	var lines []string

	lines = append(lines, fmt.Sprintf("# Summary for %s/%s over the past %d days", org, repo, days))
	lines = append(lines, fmt.Sprintf("* Workflows checked: %d", workflowsChecked))
	lines = append(lines, fmt.Sprintf("* Runs checked: %d", runsChecked))
	lines = append(lines, fmt.Sprintf("* Jobs checked: %d", jobsChecked))
	lines = append(lines, fmt.Sprintf("* Jobs missing logs: %d", jobsMissingLogs))
	lines = append(lines, "")

	for _, workflow := range tracker.workflows {
		if !workflow.hasJobsMissingLogs {
			continue
		}

		lines = append(lines, fmt.Sprintf("* [%s](%s) (%d runs):", workflow.workflow.GetName(), workflow.workflow.GetHTMLURL(), len(workflow.runs)))
		for _, run := range workflow.runs {
			if len(run.jobsMissingLogs) == 0 {
				continue
			}

			for _, job := range run.jobsMissingLogs {
				lines = append(lines, fmt.Sprintf("  * %s run [%s](%s), job [%s](%s)", job.GetStartedAt().String(), run.run.GetName(), run.run.GetHTMLURL(), job.GetName(), job.GetHTMLURL()))
			}
		}
	}

	return strings.Join(lines, "\n")
}

func processRepo(ctx context.Context, client *github.Client, org, repo string, opts githubOpts) (*repoTracker, error) {
	tracker := &repoTracker{
		org:  org,
		repo: repo,
	}

	workflows := workflowRetriever(ctx, client, org, repo)
	for workflow := range workflows.All() {
		workflowTracker, err := processWorkflow(ctx, client, org, repo, workflow, opts)
		if err != nil {
			return tracker, fmt.Errorf("failed to process workflow %d: %w", workflow.GetID(), err)
		}

		tracker.workflows = append(tracker.workflows, workflowTracker)
	}

	if err := workflows.Err(); err != nil {
		return tracker, fmt.Errorf("failed to retrieve all workflows in repo: %w", err)
	}

	return tracker, nil
}

func processWorkflow(ctx context.Context, client *github.Client, org, repo string, workflow *github.Workflow, opts githubOpts) (*workflowTracker, error) {
	tracker := &workflowTracker{
		workflow: workflow,
	}

	workflowID := workflow.GetID()
	if workflowID == 0 {
		return tracker, nil
	}

	runs := failedRunRetriever(ctx, client, org, repo, workflowID, opts.daysToCheck)
	for run := range runs.All() {
		tracker.runsChecked++

		runTracker, err := processRun(ctx, client, org, repo, run, opts)
		if err != nil {
			return tracker, fmt.Errorf("failed to process workflow run %d failed: %w", run.GetID(), err)
		}

		if len(runTracker.jobsMissingLogs) > 0 {
			// This makes queries easier later
			tracker.hasJobsMissingLogs = true
		}

		tracker.runs = append(tracker.runs, runTracker)
	}

	if err := runs.Err(); err != nil {
		return tracker, fmt.Errorf("failed to retrieve all workflow runs: %w", err)
	}

	return tracker, nil
}

func processRun(ctx context.Context, client *github.Client, org, repo string, run *github.WorkflowRun, opts githubOpts) (*runTracker, error) {
	tracker := &runTracker{
		run: run,
	}

	runID := run.GetID()
	if runID == 0 {
		return tracker, nil
	}

	jobs := jobRetriever(ctx, client, org, repo, runID)
	errCount := 0
	for job := range jobs.All() {
		tracker.jobsChecked++

		matched, err := didJobFail(ctx, client, org, repo, job, opts.includeSelfHosted)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to check if job %d failed: %v", job.GetID(), err)
			errCount++

			// Tolerate up to three consecutive errors before outright failing
			if errCount > 3 {
				return tracker, fmt.Errorf("failed to check if job %d failed: %w", job.GetID(), err)
			}
		}

		if !matched {
			continue
		}

		tracker.jobsMissingLogs = append(tracker.jobsMissingLogs, job)
	}

	if err := jobs.Err(); err != nil {
		return tracker, fmt.Errorf("failed to retrieve all jobs for workflow run %d: %w", runID, err)
	}

	return tracker, nil
}

func didJobFail(ctx context.Context, client *github.Client, org, repo string, job *github.WorkflowJob, includeSelfHosted bool) (bool, error) {
	passingStatuses := []string{
		"success",
		"neutral",
		"skipped",
		"cancelled",
		"action_required",
	}

	if slices.Contains(passingStatuses, job.GetConclusion()) {
		return false, nil
	}

	if !includeSelfHosted && slices.Contains(job.Labels, "self-hosted") {
		return false, nil
	}

	jobID := job.GetID()

	url, resp, err := client.Actions.GetWorkflowJobLogs(ctx, org, repo, jobID, 100)
	if err == nil && url != nil {
		return false, nil
	}

	if resp == nil || resp.StatusCode != 404 {
		return false, fmt.Errorf("failed to check logs for job %q: %w", job.GetURL(), err)
	}

	return true, nil
}

func workflowRetriever(ctx context.Context, client *github.Client, org, repo string) *ghiter.Iterator[*github.Workflow, *github.ListOptions] {
	getWorkflowsFunc := func(ctx context.Context, opts *github.ListOptions) ([]*github.Workflow, *github.Response, error) {
		workflows, response, err := client.Actions.ListWorkflows(ctx, org, repo, opts)
		if workflows == nil {
			return nil, response, err
		}

		return workflows.Workflows, response, err
	}

	return ghiter.NewFromFn(rateLimitRetry(getWorkflowsFunc)).Ctx(ctx)
}

func failedRunRetriever(ctx context.Context, client *github.Client, org, repo string, workflowID int64, daysToCheck int) *ghiter.Iterator[*github.WorkflowRun, *github.ListWorkflowRunsOptions] {
	getRunsFunc := func(ctx context.Context, opts *github.ListWorkflowRunsOptions) ([]*github.WorkflowRun, *github.Response, error) {
		runs, response, err := client.Actions.ListWorkflowRunsByID(ctx, org, repo, workflowID, opts)
		if runs == nil {
			return nil, response, err
		}

		return runs.WorkflowRuns, response, err
	}

	startDate := time.Now().AddDate(0, 0, -daysToCheck)
	opts := &github.ListWorkflowRunsOptions{
		Status:  "failure",
		Created: ">" + startDate.Format("2006-01-02"),
	}

	return ghiter.NewFromFn(rateLimitRetry(getRunsFunc)).Ctx(ctx).Opts(opts)
}

func jobRetriever(ctx context.Context, client *github.Client, org, repo string, runID int64) *ghiter.Iterator[*github.WorkflowJob, *github.ListWorkflowJobsOptions] {
	getJobsFunc := func(ctx context.Context, opts *github.ListWorkflowJobsOptions) ([]*github.WorkflowJob, *github.Response, error) {
		jobs, response, err := client.Actions.ListWorkflowJobs(ctx, org, repo, runID, opts)
		if jobs == nil {
			return nil, response, err
		}

		return jobs.Jobs, response, err
	}

	opts := &github.ListWorkflowJobsOptions{
		Filter: "all",
	}

	return ghiter.NewFromFn(rateLimitRetry(getJobsFunc)).Ctx(ctx).Opts(opts)
}

func rateLimitRetry[T, O any](fn func(ctx context.Context, opt O) ([]T, *github.Response, error)) func(ctx context.Context, opt O) ([]T, *github.Response, error) {
	return func(ctx context.Context, opt O) ([]T, *github.Response, error) {
		for {
			result, response, err := fn(ctx, opt)
			if err != nil {
				var rateErr *github.RateLimitError
				if errors.As(err, &rateErr) {
					// Wait for 3 second after the rate limit expires
					sleepDuration := time.Until(rateErr.Rate.Reset.Add(3 * time.Second))
					fmt.Fprintf(os.Stderr, "Rate limit hit, sleeping %s\n", sleepDuration.Round(time.Second).String())
					retryTicker := time.NewTicker(sleepDuration)

					select {
					case <-ctx.Done():
						return nil, nil, ctx.Err()
					case <-retryTicker.C:
						continue
					}
				}
			}

			return result, response, err
		}
	}
}
