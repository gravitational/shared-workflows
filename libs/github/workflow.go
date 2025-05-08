package github

import "context"

type WorkflowRunInfo struct {
	WorkflowID int64
	Name       string
	HTMLURL    string
}

func (c *Client) GeWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (WorkflowRunInfo, error) {
	workflow, _, err := c.client.Actions.GetWorkflowRunByID(ctx, org, repo, runID)
	if err != nil {
		return WorkflowRunInfo{}, err
	}

	var info WorkflowRunInfo
	info.WorkflowID = workflow.GetID()
	info.Name = workflow.GetName()
	info.HTMLURL = workflow.GetHTMLURL()

	return info, nil
}
