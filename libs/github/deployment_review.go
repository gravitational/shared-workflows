package github

import (
	"context"
	"fmt"
	"io"

	"github.com/google/go-github/v71/github"
)

type PendingDeploymentApprovalState string

const (
	PendingDeploymentApprovalStateApproved PendingDeploymentApprovalState = "approved"
	PendingDeploymentApprovalStateRejected PendingDeploymentApprovalState = "rejected"
)

// ReviewDeploymentProtectionRule reviews a deployment protection rule.
// This is used by GitHub Apps that are configured for environment protection rules.
func (c *Client) ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, runID int64, state PendingDeploymentApprovalState, envName, comment string) error {
	resp, err := c.client.Actions.ReviewCustomDeploymentProtectionRule(ctx, org, repo, runID, &github.ReviewCustomDeploymentProtectionRuleRequest{
		State:           string(state),
		EnvironmentName: envName,
		Comment:         comment,
	})

	// Sometimes the error can be eaten by the underlying client library.
	// This is a workaround to get the error from the response body.
	if err != nil {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response body: %w", err)
		}
		return fmt.Errorf("unexpected response %q: %w", body, err)
	}

	return nil
}
