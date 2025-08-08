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

package githubreconcile

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/service"
	"github.com/gravitational/teleport/api/types"
)

// WaitingWorkflowsReconciler runs a loop that periodically checks for waiting Workflow Runs and attempts to reconcile if needed.
// It will find pending Deployment Protection Rules that are causing the Workflow Runs to wait, and will attempt to sync the
// state of Access Requests with the state of GitHub Deployment Protection Rules.
//
// The main motivation is to provide fault-tolerance since the events are ephemeral and failures to process results in a loss of data.
type WaitingWorkflowsReconciler struct {
	// Configuration fields
	org                    string
	repo                   string
	reconciliationInterval time.Duration
	teleportUser           string

	// External dependencies
	ghClient   ghClient
	teleClient teleClient

	// Event handlers
	deploymentReviewEventProcessor githubevents.GitHubEventProcessor
	accessRequestReviewHandler     accessrequest.AccessRequestReviewedHandler

	// Infrastructure
	log *slog.Logger
}

// teleClient is a subset of the Teleport client interface needed for reconciliation.
// This interface allows for easier testing by providing a minimal surface area.
type teleClient interface {
	GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error)
}

// ghClient is a subset of the GitHub client interface needed for reconciliation.
// This interface allows for easier testing by providing a minimal surface area.
type ghClient interface {
	ListWaitingWorkflowRuns(ctx context.Context, org, repo string) ([]github.WorkflowRunInfo, error)
	GetPendingDeployments(ctx context.Context, org, repo string, runID int64) ([]github.PendingDeploymentInfo, error)
}

// Opt is a functional option for configuring the Reconciler.
type Opt func(*WaitingWorkflowsReconciler) error

// WithLogger sets the logger for the Reconciler.
func WithLogger(logger *slog.Logger) Opt {
	return func(r *WaitingWorkflowsReconciler) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		r.log = logger
		return nil
	}
}

// Config contains the configuration required to create a WorkflowStateReconciler.
type Config struct {
	// GitHub configuration
	Org          string
	Repo         string
	GitHubClient ghClient

	// Teleport configuration
	TeleportUser   string
	TeleportClient teleClient

	// Event handlers
	AccessRequestReviewHandler     accessrequest.AccessRequestReviewedHandler
	DeploymentReviewEventProcessor githubevents.GitHubEventProcessor
}

// validate checks that all required configuration fields are set.
func (c Config) validate() error {
	if c.Org == "" {
		return fmt.Errorf("org is required")
	}
	if c.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if c.GitHubClient == nil {
		return fmt.Errorf("GitHubClient is required")
	}
	if c.TeleportUser == "" {
		return fmt.Errorf("teleportUser is required")
	}
	if c.TeleportClient == nil {
		return fmt.Errorf("TeleportClient is required")
	}
	if c.AccessRequestReviewHandler == nil {
		return fmt.Errorf("AccessRequestReviewHandler is required")
	}
	if c.DeploymentReviewEventProcessor == nil {
		return fmt.Errorf("DeploymentReviewEventProcessor is required")
	}
	return nil
}

// NewWaitingWorkflowReconciler creates a new WorkflowStateReconciler with the provided configuration.
func NewWaitingWorkflowReconciler(config Config, opts ...Opt) (*WaitingWorkflowsReconciler, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	r := &WaitingWorkflowsReconciler{
		org:                            config.Org,
		repo:                           config.Repo,
		reconciliationInterval:         time.Second * 30, // Default to 30 seconds if not set
		ghClient:                       config.GitHubClient,
		teleClient:                     config.TeleportClient,
		deploymentReviewEventProcessor: config.DeploymentReviewEventProcessor,
		accessRequestReviewHandler:     config.AccessRequestReviewHandler,
		log:                            slog.Default().With("component", "github-reconcile"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return r, nil
}

// Run starts the reconciliation loop.
// It will run indefinitely until the context is cancelled.
func (r *WaitingWorkflowsReconciler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.reconciliationInterval):
			if err := r.reconcile(ctx); err != nil {
				r.log.Error("failed to reconcile GitHub deployment protection rules", "error", err)
			}
		}
	}
}

// reconcile performs a single reconciliation pass, checking for workflow/access request mismatches.
func (r *WaitingWorkflowsReconciler) reconcile(ctx context.Context) error {
	waitingWorkflowRuns, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return fmt.Errorf("listing waiting workflow runs: %w", err)
	}

	if len(waitingWorkflowRuns) == 0 {
		r.log.Debug("no waiting workflow runs found")
		return nil
	}

	r.log.Info("found waiting workflow runs", "count", len(waitingWorkflowRuns))

	accessRequests, err := r.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{
		User: r.teleportUser,
	})
	if err != nil {
		return fmt.Errorf("getting access requests: %w", err)
	}

	accessRequestsByWorkflowRunID := r.indexAccessRequestsByWorkflowRunID(accessRequests)

	for _, workflowRun := range waitingWorkflowRuns {
		// Check for an existing Access Request for this workflow run.
		if accessRequest, ok := accessRequestsByWorkflowRunID[workflowRun.WorkflowID]; ok {
			switch accessRequest.GetState() {
			case types.RequestState_PENDING:
				r.log.Info("waiting workflow run already has a pending access request, no action needed",
					"workflow_run", workflowRun, "access_request_id", accessRequest.GetName())
			case types.RequestState_APPROVED, types.RequestState_DENIED:
				// Access request is approved or denied, but we have a pending workflow run that hasn't had a decision made yet.
				if err := r.accessRequestReviewHandler.HandleAccessRequestReviewed(ctx, accessRequest); err != nil {
					r.log.Error("failed to handle review", "error", err)
				}
			default:
				r.log.Error("unexpected access request state",
					"state", accessRequest.GetState(), "access_request_id", accessRequest.GetName(), "workflow_run", workflowRun)
			}
			continue
		}

		// No access request for this workflow run, refire the event to create one.
		if err := r.handleMissingAccessRequest(ctx, workflowRun); err != nil {
			r.log.Error("failed to handle missing access request", "error", err, "workflow_run", workflowRun)
		}
	}

	return nil
}

// handleMissingAccessRequest will construct a new DeploymentReviewEvent for the given workflow run and will forward it to the underlying event handler.
func (r *WaitingWorkflowsReconciler) handleMissingAccessRequest(ctx context.Context, workflowRun github.WorkflowRunInfo) error {
	// For a waiting workflow run, we need to make an extra call to determine the environment name that's being requested.
	pendingDeployments, err := r.ghClient.GetPendingDeployments(ctx, workflowRun.Organization, workflowRun.Repository, workflowRun.WorkflowID)
	if err != nil {
		return fmt.Errorf("getting pending deployments for workflow run %d: %w", workflowRun.WorkflowID, err)
	}

	// Prevent duplicate processing of environments.
	// GitHub is a bit weird in that it can have multiple pending deployments for the same environment.
	// However only one API call is needed to approve/reject ALL deployments for that environment.
	handledEnvironments := map[string]struct{}{}

	for _, deployment := range pendingDeployments {
		if _, ok := handledEnvironments[deployment.Environment]; ok {
			r.log.Debug("skipping duplicate deployment for environment", "environment", deployment.Environment, "workflow_run", workflowRun)
			continue
		}

		err := r.deploymentReviewEventProcessor.HandleDeploymentReviewEventReceived(ctx,
			githubevents.DeploymentReviewEvent{
				WorkflowID:   workflowRun.WorkflowID,
				Requester:    workflowRun.Requester,
				Organization: workflowRun.Organization,
				Repository:   workflowRun.Repository,
				Environment:  deployment.Environment,
			},
		)

		if err != nil {
			r.log.Error("failed to process deployment review event", "workflow_run", workflowRun, "environment", deployment.Environment, "error", err)
			continue
		}
		handledEnvironments[deployment.Environment] = struct{}{}
	}

	return nil
}

// indexAccessRequestsByWorkflowRunID creates a map of access requests indexed by workflow run ID.
func (r *WaitingWorkflowsReconciler) indexAccessRequestsByWorkflowRunID(accessRequests []types.AccessRequest) map[int64]types.AccessRequest {
	index := make(map[int64]types.AccessRequest, len(accessRequests))

	for _, accessRequest := range accessRequests {
		workflowInfo, err := service.GetWorkflowLabels(accessRequest)
		if err != nil {
			r.log.Warn("skipping access request with invalid workflow labels",
				"access_request_id", accessRequest.GetName(),
				"error", err)
		} else {
			index[workflowInfo.WorkflowRunID] = accessRequest
		}
	}

	r.log.Debug("indexed access requests by workflow run ID", "count", len(index))
	return index
}
