package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v71/github"
)

// PendingDeploymentApprovalState represents the state of a pending deployment approval.
type PendingDeploymentApprovalState string

const (
	// PendingDeploymentApprovalStateApproved indicates that the deployment protection rule is approved.
	PendingDeploymentApprovalStateApproved PendingDeploymentApprovalState = "approved"
	// PendingDeploymentApprovalStateRejected indicates that the deployment protection rule is rejected.
	PendingDeploymentApprovalStateRejected PendingDeploymentApprovalState = "rejected"
)

// ReviewDeploymentProtectionRuleInfo contains information about a deployment protection rule that is being reviewed.
// This is used by GitHub Apps that are configured for environment protection rules.
type ReviewDeploymentProtectionRuleInfo struct {
	// RunID is the ID of the workflow run that is being reviewed.
	RunID int64 `json:"run_id"`
	// EnvironmentName is the name of the environment that is being reviewed.
	EnvironmentName string `json:"environment_name"`
	// State is the state of the deployment protection rule.
	State PendingDeploymentApprovalState `json:"state"`
	// Comment is an optional comment for the deployment protection rule.
	Comment string `json:"comment,omitempty"`
}

func (c *Client) ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, runID int64, state PendingDeploymentApprovalState, envName, comment string) error {
	resp, err := c.client.Actions.ReviewCustomDeploymentProtectionRule(ctx, org, repo, runID, &github.ReviewCustomDeploymentProtectionRuleRequest{
		State:           string(state),
		EnvironmentName: envName,
		Comment:         comment,
	})

	if err != nil {
		// Attempt to read the response body for more details on the error.
		err = errors.Join(err, errorFromBody(resp.Body))
		return fmt.Errorf("ReviewCustomDeploymentProtectionRule API call: %w", err)
	}

	return nil
}

type PendingDeploymentInfo struct {
	Environment string
}

// GetPendingDeployments retrieves all pending deployments for a given workflow run.
func (c *Client) GetPendingDeployments(ctx context.Context, org, repo string, runID int64) ([]PendingDeploymentInfo, error) {
	data, _, err := c.client.Actions.GetPendingDeployments(ctx, org, repo, runID)
	if err != nil {
		return nil, fmt.Errorf("getting pending deployments: %w", err)
	}

	pending := []PendingDeploymentInfo{}
	for _, deployment := range data {
		pending = append(pending, PendingDeploymentInfo{
			Environment: deployment.GetEnvironment().GetName(),
		})
	}

	return pending, nil
}
