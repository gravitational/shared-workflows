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

package service

import (
	"context"
	"fmt"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// reconcileWaitingWorkflows performs a single reconciliation pass for all waiting workflows.
func (r *ReleaseService) reconcileWaitingWorkflows(ctx context.Context) {
	unhandledDeploymentProtectionRule, unhandledAccessRequest, err := r.findReconciliationWork(ctx)
	if err != nil {
		r.log.Error("failed to find waiting workflows needing reconcile", "error", err)
		return
	}

	if len(unhandledDeploymentProtectionRule) == 0 && len(unhandledAccessRequest) == 0 {
		return
	}

	for _, rule := range unhandledDeploymentProtectionRule {
		r.log.Info("found deployment protection rule needing reconcile", "deployment_protection_rule", rule)
		if err := r.HandleDeploymentReviewEventReceived(ctx, rule); err != nil {
			r.log.Error("reconciling deployment review event", "deployment_protection_rule", rule, "error", err)
		}
	}

	for _, accessRequest := range unhandledAccessRequest {
		r.log.Info("found access request needing reconcile", "access_request", accessRequest.GetName())
		if err := r.HandleAccessRequestReviewed(ctx, accessRequest); err != nil {
			r.log.Error("reconciling access request reviewed", "access_request", accessRequest.GetName(), "error", err)
		}
	}
}

// findReconciliationWork finds all waiting workflows and access requests that need to be reconciled.
// It will find pending Deployment Protection Rules that are causing the Workflow Runs to wait, and will attempt to sync the
// state of Access Requests with the state of GitHub Deployment Protection Rules.
//
// The main motivation is to provide fault-tolerance since the events are ephemeral and failures to process results in a loss of data.
func (r *ReleaseService) findReconciliationWork(ctx context.Context) ([]githubevents.DeploymentReviewEvent, []types.AccessRequest, error) {
	waitingWorkflowRuns, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return nil, nil, fmt.Errorf("listing waiting workflow runs: %w", err)
	}

	if len(waitingWorkflowRuns) == 0 {
		return nil, nil, nil
	}

	accessRequests, err := r.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{
		User: r.teleportUser,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("getting access requests: %w", err)
	}

	accessRequestsByWorkflowRunID := r.indexAccessRequestsByWorkflowRunID(accessRequests)

	unhandledDeploymentProtectionRule := make([]githubevents.DeploymentReviewEvent, 0, len(waitingWorkflowRuns))
	unhandledAccessRequests := make([]types.AccessRequest, 0, len(accessRequestsByWorkflowRunID))
	for _, workflowRun := range waitingWorkflowRuns {
		// Check for an existing Access Request for this workflow run.
		if accessRequest, ok := accessRequestsByWorkflowRunID[workflowRun.WorkflowID]; ok {
			switch accessRequest.GetState() {
			case types.RequestState_PENDING:
			case types.RequestState_APPROVED, types.RequestState_DENIED:
				// Access request is approved or denied, but we have a pending workflow run that hasn't had a decision made yet.
				unhandledAccessRequests = append(unhandledAccessRequests, accessRequest)
			default:
				r.log.Error("unexpected access request state",
					"state", accessRequest.GetState(), "access_request_id", accessRequest.GetName(), "workflow_run", workflowRun)
			}
			continue
		}

		newEvents, err := r.constructNewEventsForWorkflow(ctx, workflowRun)
		if err != nil {
			return nil, nil, fmt.Errorf("constructing new events for workflow run %d: %w", workflowRun.WorkflowID, err)
		}
		unhandledDeploymentProtectionRule = append(unhandledDeploymentProtectionRule, newEvents...)
	}

	return unhandledDeploymentProtectionRule, unhandledAccessRequests, nil
}

// constructNewEventsForWorkflow constructs new DeploymentReviewEvent for each waiting workflow run
func (r *ReleaseService) constructNewEventsForWorkflow(ctx context.Context, workflowRun github.WorkflowRunInfo) ([]githubevents.DeploymentReviewEvent, error) {
	// For a waiting workflow run, we need to make an extra call to determine the environment name that's being requested.
	pendingDeployments, err := r.ghClient.GetPendingDeployments(ctx, workflowRun.Organization, workflowRun.Repository, workflowRun.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("getting pending deployments for workflow run %d: %w", workflowRun.WorkflowID, err)
	}

	// Prevent duplicate processing of environments.
	// GitHub is a bit weird in that it can have multiple pending deployments for the same environment.
	// However only one API call is needed to approve/reject ALL deployments for that environment.
	handledEnvironments := map[string]struct{}{}

	newEvents := []githubevents.DeploymentReviewEvent{}
	for _, deployment := range pendingDeployments {
		if _, ok := handledEnvironments[deployment.Environment]; ok {
			continue
		}

		newEvents = append(newEvents, githubevents.DeploymentReviewEvent{
			WorkflowID:   workflowRun.WorkflowID,
			Requester:    workflowRun.Requester,
			Organization: workflowRun.Organization,
			Repository:   workflowRun.Repository,
			Environment:  deployment.Environment,
		})

		handledEnvironments[deployment.Environment] = struct{}{}
	}

	return newEvents, nil
}

// indexAccessRequestsByWorkflowRunID creates a map of access requests indexed by workflow run ID.
func (r *ReleaseService) indexAccessRequestsByWorkflowRunID(accessRequests []types.AccessRequest) map[int64]types.AccessRequest {
	index := make(map[int64]types.AccessRequest, len(accessRequests))

	for _, accessRequest := range accessRequests {
		workflowInfo, err := GetWorkflowLabels(accessRequest)
		if err != nil {
			r.log.Warn("skipping access request with invalid workflow labels",
				"access_request_id", accessRequest.GetName(),
				"error", err)
			continue
		}

		index[workflowInfo.WorkflowRunID] = accessRequest
	}

	return index
}
