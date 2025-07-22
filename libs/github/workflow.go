package github

import (
	"context"
	"fmt"

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

// GeWorkflowRunInfo retrieves information about a specific workflow run by its ID.
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
		return nil, err
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
