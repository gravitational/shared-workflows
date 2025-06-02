package github

import (
	"context"

	go_github "github.com/google/go-github/v71/github"
)

type WorkflowRunInfo struct {
	WorkflowID   int64
	Name         string
	HTMLURL      string
	Requester    string
	Organization string
	Repository   string
}

func (c *Client) GeWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (WorkflowRunInfo, error) {
	workflow, _, err := c.client.Actions.GetWorkflowRunByID(ctx, org, repo, runID)
	if err != nil {
		return WorkflowRunInfo{}, err
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
