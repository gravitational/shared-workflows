package github

import (
	"context"
	"fmt"
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
}

// GeWorkflowRunInfo retrieves information about a specific workflow run by its ID.
func (c *Client) GeWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (WorkflowRunInfo, error) {
	workflow, _, err := c.client.Actions.GetWorkflowRunByID(ctx, org, repo, runID)
	if err != nil {
		return WorkflowRunInfo{}, fmt.Errorf("GetWorkflowRunByID API call: %w", err)
	}

	return WorkflowRunInfo{
		WorkflowID: workflow.GetID(),
		Name:       workflow.GetName(),
		HTMLURL:    workflow.GetHTMLURL(),
	}, nil
}
