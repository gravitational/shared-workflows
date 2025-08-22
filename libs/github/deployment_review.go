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
	"errors"
	"fmt"
	"net/http"

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

var (
	// ErrNoPendingDeployments is returned when there are no pending deployments for a given workflow run.
	// This can happen if the workflow run has already been approved or rejected which clears the pending deployments.
	// This can also happen if the workflow has stopped executing due to cancellation, failure, or completion.
	ErrNoPendingDeployments = errors.New("no pending deployments for the given workflow run")
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

// ReviewDeploymentProtectionRule reviews a deployment protection rule.
// This is used by GitHub Apps that are configured for environment protection rules.
//
// If an error occurs, this function can return sentinel errors that callers can check to handle specific error conditions gracefully.
// For example, if there are no pending deployments for the given workflow run, it will return [ErrNoPendingDeployments].
// In some cases this is not a fail-state, and the caller can choose to ignore it.
func (c *Client) ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, info ReviewDeploymentProtectionRuleInfo) error {
	resp, err := c.client.Actions.ReviewCustomDeploymentProtectionRule(ctx, org, repo, info.RunID, &github.ReviewCustomDeploymentProtectionRuleRequest{
		State:           string(info.State),
		EnvironmentName: info.EnvironmentName,
		Comment:         info.Comment,
	})

	if err != nil {
		errBody := errorFromBody(resp.Body)
		switch resp.Response.StatusCode {
		case http.StatusUnprocessableEntity:
			// This case can occur if there are no pending deployments for the given run ID.
			return errors.Join(ErrNoPendingDeployments, errBody)
		default:
			return fmt.Errorf("ReviewCustomDeploymentProtectionRule API call: %w", errors.Join(err, errBody))
		}
	}

	return nil
}

// PendingDeploymentInfo contains information about a pending deployment that is waiting for approval.
// The API returns environment information for the pending deployment as well as other metadata.
// Only the environment name is currently useful, so this is a simplified version of the data returned by the API.
type PendingDeploymentInfo struct {
	// Environment is the name of the environment that the workflow run is waiting for approval on.
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
