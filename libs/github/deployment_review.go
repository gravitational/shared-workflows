package github

import (
	"context"
	"fmt"
	"log/slog"

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

type PendingDeploymentInfo struct {
	Org     string
	Repo    string
	RunID   int64
	EnvIDs  []int64
	State   PendingDeploymentApprovalState
	Comment string
}

func (c *Client) UpdatePendingDeployment(ctx context.Context, info PendingDeploymentInfo) ([]Deployment, error) {
	objs, _, err := c.client.Actions.PendingDeployments(ctx, info.Org, info.Repo, info.RunID, &github.PendingDeploymentsRequest{
		State:          string(info.State),
		EnvironmentIDs: info.EnvIDs,
		Comment:        info.Comment,
	})

	if err != nil {
		slog.Default().Error("failed to update pending deployments", "pendingDeployment", info)
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

func (i *PendingDeploymentInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("org", i.Org),
		slog.String("repo", i.Repo),
		slog.Int64("run_id", i.RunID),
		slog.String("state", string(i.State)),
		slog.Any("env_ids", i.EnvIDs),
	)
}
