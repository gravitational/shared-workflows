package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v69/github"
)

func (c *Client) ApprovePendingDeploymenst(ctx context.Context, runID int64, envIDs []int64, comment string) ([]*github.Deployment, error) {
	deploys, _, err := c.client.Actions.PendingDeployments(ctx, "owner", "repo", 1, &github.PendingDeploymentsRequest{
		State:          "approved",
		EnvironmentIDs: envIDs,
		Comment:        comment,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to approve pending deployments: %w", err)
	}
	return deploys, nil
}

func (c *Client) RejectPendingDeploymenst(ctx context.Context, runID int64, envIDs []int64, comment string) ([]*github.Deployment, error) {
	deploys, _, err := c.client.Actions.PendingDeployments(ctx, "owner", "repo", 1, &github.PendingDeploymentsRequest{
		State:          "rejected",
		EnvironmentIDs: envIDs,
		Comment:        comment,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to reject pending deployments: %w", err)
	}
	return deploys, nil
}
