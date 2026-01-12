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

package github

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	go_github "github.com/google/go-github/v71/github"
)

// WorkflowRunInfo contains information about a specific workflow run.
type WorkflowRunInfo struct {
	// WorkflowID is the unique identifier for the workflow run.
	WorkflowID int64
	// Name is the name of the workflow run.
	// This is typically defined in the workflow YAML file.
	Name string
	// HTMLURL is the URL to view the workflow run on GitHub.
	// This is useful for linking to the run in user interfaces or logs.
	HTMLURL string
	// Requester is the GitHub username of the user who triggered the workflow run.
	// This can be useful for auditing or tracking purposes.
	Requester string
	// Organization is the GitHub organization that owns the repository.
	Organization string
	// Repository is the name of the repository where the workflow run occurred.
	Repository string
}

// GetWorkflowRunInfo retrieves information about a specific workflow run by its ID.
func (c *Client) GetWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (WorkflowRunInfo, error) {
	workflow, _, err := c.client.Actions.GetWorkflowRunByID(ctx, org, repo, runID)
	if err != nil {
		return WorkflowRunInfo{}, fmt.Errorf("GetWorkflowRunByID API call: %w", err)
	}

	return WorkflowRunInfo{
		WorkflowID:   workflow.GetID(),
		Name:         workflow.GetName(),
		HTMLURL:      workflow.GetHTMLURL(),
		Requester:    workflow.GetActor().GetLogin(),
		Organization: org,
		Repository:   repo,
	}, nil
}

// ListWaitingWorkflowRuns lists all workflow runs in a repository that are currently waiting for approval.
// For example, this can be used to list all workflow runs that are waiting for a deployment review.
func (c *Client) ListWaitingWorkflowRuns(ctx context.Context, org, repo string) ([]WorkflowRunInfo, error) {
	data, _, err := c.client.Actions.ListRepositoryWorkflowRuns(ctx, org, repo, &go_github.ListWorkflowRunsOptions{
		Status: "waiting",
	})
	if err != nil {
		return nil, fmt.Errorf("listing waiting workflow runs: %w", err)
	}

	allRuns := []WorkflowRunInfo{}
	for _, run := range data.WorkflowRuns {
		allRuns = append(allRuns, WorkflowRunInfo{
			WorkflowID:   run.GetID(),
			Name:         run.GetName(),
			HTMLURL:      run.GetHTMLURL(),
			Requester:    run.GetActor().GetLogin(),
			Organization: org,
			Repository:   repo,
		})
	}

	return allRuns, nil
}

// WorkflowDispatchRequest contains the parameters for triggering a workflow dispatch event.
type WorkflowDispatchRequest struct {
	// WorkflowName is the name of the workflow to run.
	// Workflows are defined in the .github/workflows directory of a repository and are named by their filename .github/workflows/<NAME>.
	// Note that the file extension (.yml or .yaml) should be included when specifying the workflow name.
	WorkflowName string
	// Ref is the git reference for the workflow dispatch (branch, tag, or commit SHA).
	Ref string
	// Inputs are the input parameters for the workflow dispatch.
	// Great care should be taken to ensure that no sensitive information or malicious data is passed via inputs.
	// Inputs should be validated and sanitized before being used in the workflow. Never pass untrusted data via inputs.
	Inputs map[string]any
}

// WorkflowDispatch triggers a workflow dispatch event.
// Care should be taken when using this function as it can trigger arbitrary workflows in the target repository.
// Ensure that the workflow being triggered is safe and does not execute untrusted code.
// Always validate and sanitize any inputs passed to the workflow.
func (c *Client) WorkflowDispatch(ctx context.Context, org, repo string, req WorkflowDispatchRequest) error {
	if req.WorkflowName == "" {
		return fmt.Errorf("workflow name is required")
	}
	if req.Ref == "" {
		return fmt.Errorf("ref is required")
	}

	_, err := c.client.Actions.CreateWorkflowDispatchEventByFileName(ctx, org, repo, req.WorkflowName, go_github.CreateWorkflowDispatchEventRequest{
		Ref:    req.Ref,
		Inputs: req.Inputs,
	})
	if err != nil {
		return fmt.Errorf("CreateWorkflowDispatchEventByFileName API call: %w", err)
	}
	return err
}

// WorkflowRunNotFoundError is a sentinel error returned when a workflow run cannot be found.
// This is typically used for tying workflow dispatch events to their resulting workflow runs.
// We have to use heuristic based searching to be able to reliably find the correct run.
// So this error indicates that our search did not yield any results, but may succeed if retried later.
type WorkflowRunNotFoundError struct {
	Message string
}

func (e *WorkflowRunNotFoundError) Error() string {
	return e.Message
}

func newWorkflowRunNotFoundError(message string) *WorkflowRunNotFoundError {
	return &WorkflowRunNotFoundError{
		Message: message,
	}
}

// FindWorkflowRunIDByUniqueStepNameRequest contains the parameters for finding a workflow run ID by a unique step name.
type FindWorkflowRunIDByUniqueStepNameRequest struct {
	// WorkflowName is the name of the workflow to search within.
	// Workflows are defined in the .github/workflows directory of a repository and are named by their filename .github/workflows/<NAME>.
	// Note that the file extension (.yml or .yaml) should be included when specifying the workflow name.
	WorkflowName string
	// StepName is the unique name of the step to search for within the workflow runs.
	StepName string
}

// FindWorkflowRunIDByUniqueStepName finds a workflow run ID by searching for a unique step name within the workflow runs of a repository.
// Workflow Dispatch events can be used to trigger workflows but do not return the workflow run ID directly.
// This function can be used as a workaround to find the workflow run ID after triggering a workflow dispatch.
// For this to work, the workflow must include a step with a unique name that can be searched for.
//
// It returns the workflow run ID if found, or an error if not found or if any issues occur during the search.
// The error can be of type [WorkflowRunNotFoundError] if the step name is not found in any workflow run.
// This error is retryable as the workflow run may not be ready yet.
func (c *Client) FindWorkflowRunIDByUniqueStepName(ctx context.Context, org, repo string, req FindWorkflowRunIDByUniqueStepNameRequest) (WorkflowRunInfo, error) {
	if req.WorkflowName == "" {
		return WorkflowRunInfo{}, fmt.Errorf("workflow name is required")
	}
	if req.StepName == "" {
		return WorkflowRunInfo{}, fmt.Errorf("step name is required")
	}

	runs, _, err := c.client.Actions.ListWorkflowRunsByFileName(ctx, org, repo, req.WorkflowName, &go_github.ListWorkflowRunsOptions{
		ListOptions: go_github.ListOptions{
			PerPage: 100,
		},
	})
	if err != nil {
		return WorkflowRunInfo{}, fmt.Errorf("listing workflow runs: %w", err)
	}

	for _, run := range runs.WorkflowRuns {
		// Pending runs do not have steps yet, so skip them
		if run.GetStatus() == "pending" {
			continue
		}

		jobs, _, err := c.client.Actions.ListWorkflowJobs(ctx, org, repo, run.GetID(), &go_github.ListWorkflowJobsOptions{
			ListOptions: go_github.ListOptions{
				PerPage: 100,
			},
		})
		if err != nil {
			return WorkflowRunInfo{}, fmt.Errorf("listing workflow jobs for run ID %d: %w", run.GetID(), err)
		}

		for _, job := range jobs.Jobs {
			for _, step := range job.Steps {
				if strings.Contains(step.GetName(), req.StepName) { // Happy path: found a matching step
					return workflowRunInfoFromObj(run), nil
				}
			}
		}

	}

	// If we reach here, we did not find the step in any run
	return WorkflowRunInfo{}, newWorkflowRunNotFoundError(fmt.Sprintf("could not find workflow run for %q with step name %q", req.WorkflowName, req.StepName))
}

// workflowRunInfoFromObj converts a [go_github.WorkflowRun] object to a [WorkflowRunInfo].
func workflowRunInfoFromObj(githubObj *go_github.WorkflowRun) WorkflowRunInfo {
	return WorkflowRunInfo{
		WorkflowID:   githubObj.GetID(),
		Name:         githubObj.GetName(),
		HTMLURL:      githubObj.GetHTMLURL(),
		Requester:    githubObj.GetActor().GetLogin(),
		Organization: githubObj.GetRepository().GetOwner().GetLogin(),
		Repository:   githubObj.GetRepository().GetName(),
	}
}

// LogValue implements [slog.LogValuer] for WorkflowRunInfo to provide structured logging.
func (w WorkflowRunInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("organization", w.Organization),
		slog.String("repository", w.Repository),
		slog.Int64("workflow_id", w.WorkflowID),
	)
}
