package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v69/github"
)

type PendingDeploymentApprovalState string

const (
	PendingDeploymentApprovalStateApproved PendingDeploymentApprovalState = "approved"
	PendingDeploymentApprovalStateRejected PendingDeploymentApprovalState = "rejected"
)

type Deployment struct {
	URL           string
	ID            int64
	SHA           string
	Ref           string
	Task          string
	Environment   string
	Description   string
	StatusesURL   string
	RepositoryURL string
	NodeID        string
}

type PendingDeploymentOpts struct {
	EnvIDs  []int64
	Comment string
}

func (c *Client) UpdatePendingDeployment(ctx context.Context, org, repo string, runID int64, state PendingDeploymentApprovalState, opts *PendingDeploymentOpts) ([]Deployment, error) {
	objs, _, err := c.client.Actions.PendingDeployments(ctx, org, repo, runID, &github.PendingDeploymentsRequest{
		State:          string(state),
		EnvironmentIDs: opts.EnvIDs,
		Comment:        opts.Comment,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to update pending deployments: %w", err)
	}

	var deploys []Deployment
	for _, obj := range objs {
		deploys = append(deploys, Deployment{
			URL:           obj.GetURL(),
			ID:            obj.GetID(),
			SHA:           obj.GetSHA(),
			Ref:           obj.GetRef(),
			Task:          obj.GetTask(),
			Environment:   obj.GetEnvironment(),
			Description:   obj.GetDescription(),
			StatusesURL:   obj.GetStatusesURL(),
			RepositoryURL: obj.GetRepositoryURL(),
			NodeID:        obj.GetNodeID(),
		})
	}

	return deploys, nil
}
