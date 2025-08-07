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

// Reconciler is a service that reconciles the state of GitHub deployment protection rules with the state of Teleport access requests.
// Events are ephemeral and failures to process results in a loss of data. This serves as a redundancy for such cases.
// To recover from this, the reconciler will periodically check the state of the deployment protection rules and the state of the access requests.
// If it detects a mismatch, it will fire an event to update the state of the access request.
type Reconciler struct {
	deployReviewEventProcessor githubevents.GitHubEventProcessor
	reviewHandler              accessrequest.AccessRequestReviewedHandler

	org  string
	repo string

	// ghClient is the GitHub client used to interact with the GitHub API.
	ghClient github.Client
	// teleClient is the Teleport client used to interact with the Teleport API.
	teleClient teleClient

	log *slog.Logger
}

// Small interface to allow for easier testing of the Teleport client.
// This is a subset of the teleport.Client interface that we need for our purposes.
// It is not intended to be a complete representation of the Teleport API or the teleport.Client implementation.
type teleClient interface {
	GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error)
}

// Opt is a functional option for configuring the Reconciler.
type Opt func(*Reconciler) error

// WithLogger sets the logger for the Reconciler.
func WithLogger(logger *slog.Logger) Opt {
	return func(r *Reconciler) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		r.log = logger
		return nil
	}
}

func NewReconciler(ghClient github.Client, teleClient teleClient, org, repo string, reviewHandler accessrequest.AccessRequestReviewedHandler, deployProcessor githubevents.GitHubEventProcessor, opts ...Opt) (*Reconciler, error) {
	r := &Reconciler{
		deployReviewEventProcessor: deployProcessor,
		reviewHandler:              reviewHandler,
		org:                        org,
		repo:                       repo,
		ghClient:                   ghClient,
		teleClient:                 teleClient,
		log:                        slog.Default().With("component", "github-reconciler"),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return r, nil
}

func (r *Reconciler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
			if err := r.reconcile(ctx); err != nil {
				r.log.Error("failed to reconcile GitHub deployment protection rules", "error", err)
			}
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) error {
	// Reconciler is driven by the state of GitHub workflow pendingWorkflows
	// If there are pending workflow pendingWorkflows, we need to investigate the state of their corresponding Access Request.
	pendingWorkflows, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return fmt.Errorf("failed to list pending deployment protection rules: %w", err)
	}

	if len(pendingWorkflows) == 0 {
		// No pending deployment protection rules, nothing to reconcile.
		return nil
	}

	// Gather Access Requests that are relevant to the workflow runs.
	accessRequests, err := r.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{
		// TODO: Figure out filtering for Access Requests
	})
	if err != nil {
		return fmt.Errorf("failed to get access requests: %w", err)
	}

	accessRequestsByWorkflowRun := r.buildWorkflowRunToAccessRequestMap(accessRequests)

	for _, pendingWork := range pendingWorkflows {
		if req, ok := accessRequestsByWorkflowRun[pendingWork.WorkflowID]; ok {
			// Access request exists for this workflow run, check its state.
			if req.GetState() == types.RequestState_PENDING {
				continue // Access request is still pending, nothing to do.
			}

			// Access request is not pending, and we have a pending workflow run.
			// This means we need to refire the event to update the GitHub deployment protection rule.
			r.log.Info("detected access request state change, refiring event", "workflow_run_id", pendingWork.WorkflowID, "workflow_name", pendingWork.Name, "org", pendingWork.Organization, "repo", pendingWork.Repository)
			if err := r.reviewHandler.HandleAccessRequestReviewed(ctx, req); err != nil {
				r.log.Error("failed to handle review", "error", err)
			}
			continue
		}

		// No access request for this workflow run, refire the event to create one.
		r.log.Info("detected missing access request, refiring event", "workflow_run_id", pendingWork.WorkflowID, "workflow_name", pendingWork.Name, "org", pendingWork.Organization, "repo", pendingWork.Repository)
		pendingDeploys, err := r.ghClient.GetPendingDeployments(ctx, pendingWork.Organization, pendingWork.Repository, pendingWork.WorkflowID)
		if err != nil {
			return fmt.Errorf("failed to get pending deployments for workflow run %d: %w", pendingWork.WorkflowID, err)
		}

		for _, deployment := range pendingDeploys {
			err := r.deployReviewEventProcessor.HandleDeploymentReviewEventReceived(ctx, githubevents.DeploymentReviewEvent{
				WorkflowID:   pendingWork.WorkflowID,
				Requester:    pendingWork.Requester,
				Organization: pendingWork.Organization,
				Repository:   pendingWork.Repository,
				Environment:  deployment.Environment,
			})
			if err != nil {
				r.log.Error("failed to process deployment review event", "error", err, "workflow_run_id", pendingWork.WorkflowID, "environment", deployment.Environment)
				continue
			}
		}
	}

	return nil
}

// buildWorkflowRunToAccessRequestMap returns a map of access requests indexed by their workflow run IDs.
func (r *Reconciler) buildWorkflowRunToAccessRequestMap(accessRequests []types.AccessRequest) map[int64]types.AccessRequest {
	stateMap := map[int64]types.AccessRequest{}
	for _, req := range accessRequests {
		workflowInfo, err := service.GetWorkflowLabels(req)
		if err != nil {
			r.log.Warn("failed to get workflow labels from access request", "error", err, "access_request_id", req.GetName())
			continue
		}

		stateMap[workflowInfo.WorkflowRunID] = req
	}
	return stateMap
}
