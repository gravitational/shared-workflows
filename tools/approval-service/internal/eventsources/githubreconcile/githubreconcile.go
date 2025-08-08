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

// WorkflowStateReconciler runs a loop that periodically checks the state of GitHub Deployment Protection Rules and Teleport access requests.
// It is responsible for ensuring that the state of Access Requests reflects the state of GitHub Deployment Protection Rules.
//
// The main motivation for this is that events are ephemeral and failures to process results in a loss of data.
// To provide fault-tolerance without extra infrastructure, we will periodically check the state of GitHub workflows and Teleport access requests.
// If it detects a mismatch, it will fire an event to update the state of the GitHub Deployment Protection Rule.
type WorkflowStateReconciler struct {
	// Configuration fields
	org                    string
	repo                   string
	reconciliationInterval time.Duration

	// External dependencies
	ghClient   github.Client
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

// Opt is a functional option for configuring the Reconciler.
type Opt func(*WorkflowStateReconciler) error

// WithLogger sets the logger for the Reconciler.
func WithLogger(logger *slog.Logger) Opt {
	return func(r *WorkflowStateReconciler) error {
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
	GitHubClient github.Client

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
		return fmt.Errorf("Org is required")
	}
	if c.Repo == "" {
		return fmt.Errorf("Repo is required")
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

// NewReconciler creates a new WorkflowStateReconciler with the provided configuration.
func NewReconciler(config Config, opts ...Opt) (*WorkflowStateReconciler, error) {
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	r := &WorkflowStateReconciler{
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
func (r *WorkflowStateReconciler) Run(ctx context.Context) error {
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
func (r *WorkflowStateReconciler) reconcile(ctx context.Context) error {
	waitingWorkflowRuns, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return fmt.Errorf("listing waiting workflow runs: %w", err)
	}

	if len(waitingWorkflowRuns) == 0 {
		r.log.Debug("no waiting workflow runs found")
		return nil
	}

	r.log.Info("found waiting workflow runs", "count", len(waitingWorkflowRuns))

	accessRequests, err := r.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{})
	if err != nil {
		return fmt.Errorf("getting access requests: %w", err)
	}

	accessRequestsByWorkflowRunID := r.indexAccessRequestsByWorkflowRunID(accessRequests)

	for _, workflowRun := range waitingWorkflowRuns {
		// Check for an existing Access Request for this workflow run.
		if accessRequest, ok := accessRequestsByWorkflowRunID[workflowRun.WorkflowID]; ok {
			if accessRequest.GetState() == types.RequestState_APPROVED || accessRequest.GetState() == types.RequestState_DENIED {
				// Access request is approved or denied, but we have a pending workflow run that hasn't had a decision made yet.
				// This means we need to "refire" the event and have the Access Request reviewed again.
				r.log.Info("detected access request state change, refiring event", "workflow_run_id", workflowRun.WorkflowID, "workflow_name", workflowRun.Name, "org", workflowRun.Organization, "repo", workflowRun.Repository)
				if err := r.accessRequestReviewHandler.HandleAccessRequestReviewed(ctx, accessRequest); err != nil {
					r.log.Error("failed to handle review", "error", err)
				}
			}
			continue
		}

		// No access request for this workflow run, refire the event to create one.
		r.log.Info("detected missing access request, refiring event", "workflow_run_id", workflowRun.WorkflowID, "workflow_name", workflowRun.Name, "org", workflowRun.Organization, "repo", workflowRun.Repository)
		pendingDeployments, err := r.ghClient.GetPendingDeployments(ctx, workflowRun.Organization, workflowRun.Repository, workflowRun.WorkflowID)
		if err != nil {
			return fmt.Errorf("failed to get pending deployments for workflow run %d: %w", workflowRun.WorkflowID, err)
		}

		for _, deployment := range pendingDeployments {
			err := r.deploymentReviewEventProcessor.HandleDeploymentReviewEventReceived(ctx, githubevents.DeploymentReviewEvent{
				WorkflowID:   workflowRun.WorkflowID,
				Requester:    workflowRun.Requester,
				Organization: workflowRun.Organization,
				Repository:   workflowRun.Repository,
				Environment:  deployment.Environment,
			})
			if err != nil {
				r.log.Error("failed to process deployment review event", "error", err, "workflow_run_id", workflowRun.WorkflowID, "environment", deployment.Environment)
				continue
			}
		}
	}

	return nil
}

// indexAccessRequestsByWorkflowRunID creates a map of access requests indexed by workflow run ID.
func (r *WorkflowStateReconciler) indexAccessRequestsByWorkflowRunID(accessRequests []types.AccessRequest) map[int64]types.AccessRequest {
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
