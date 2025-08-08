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
	deploymentReviewEventProcessor githubevents.GitHubEventProcessor
	accessRequestReviewHandler     accessrequest.AccessRequestReviewedHandler

	org  string
	repo string

	// ghClient is the GitHub client used to interact with the GitHub API.
	ghClient github.Client
	// teleClient is the Teleport client used to interact with the Teleport API.
	teleClient teleClient

	// reconciliationInterval is the time between reconciliation runs
	reconciliationInterval time.Duration

	log *slog.Logger
}

// teleportClient is a small interface to allow for easier testing of the Teleport client.
// This is a subset of the teleport.Client interface that we need for our purposes.
// It is not intended to be a complete representation of the Teleport API or the teleport.Client implementation.
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

// Config is the required configuration for the WorkflowStateReconciler.
type Config struct {
	// Org is the GitHub organization name to watch.
	Org string
	// Repo is the GitHub repository name to watch.
	Repo string
	// GitHubClient is the GitHub client used to interact with the GitHub API.
	GitHubClient github.Client

	// TeleportUser is the Teleport user that the Approval Service will use to interact with Teleport.
	// This is used to filter Access Requests to relevant ones for the GitHub workflows.
	TeleportUser string
	// TeleportClient is the Teleport client used to interact with the Teleport API.
	TeleportClient teleClient

	// AccessRequestReviewHandler is the handler for Access Request reviewed events.
	AccessRequestReviewHandler accessrequest.AccessRequestReviewedHandler
	// DeploymentReviewEventProcessor is the event processor for GitHub deployment review events.
	DeploymentReviewEventProcessor githubevents.GitHubEventProcessor
}

func NewReconciler(config Config, opts ...Opt) (*WorkflowStateReconciler, error) {
	r := &WorkflowStateReconciler{
		deploymentReviewEventProcessor: config.DeploymentReviewEventProcessor,
		accessRequestReviewHandler:     config.AccessRequestReviewHandler,
		org:                            config.Org,
		repo:                           config.Repo,
		ghClient:                       config.GitHubClient,
		teleClient:                     config.TeleportClient,
		reconciliationInterval:         time.Second * 30, // Default to 30 seconds

		log: slog.Default().With("component", "github-reconciler"),
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

// reconcile contains the main logic for reconciling the state of GitHub deployment protection rules with Teleport access requests.
// It is driven by the state of waiting GitHub Workflow Runs, which may be waiting due to pending Deployment Protection Rules.
// If there is a waiting workflow run, it will attempt to reconcile the state of the Deployment Protection Rule with the state of the Access Request.
func (r *WorkflowStateReconciler) reconcile(ctx context.Context) error {
	waitingWorkflowRuns, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return fmt.Errorf("listing waiting workflow runs: %w", err)
	}

	if len(waitingWorkflowRuns) == 0 {
		// No waiting deployment protection rules, nothing to reconcile.
		return nil
	}

	// Gather Access Requests that are relevant to the workflow runs.
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

// indexAccessRequestsByWorkflowRunID returns a map of access requests indexed by their workflow run IDs.
func (r *WorkflowStateReconciler) indexAccessRequestsByWorkflowRunID(accessRequests []types.AccessRequest) map[int64]types.AccessRequest {
	accessRequestIndex := map[int64]types.AccessRequest{}
	for _, accessRequest := range accessRequests {
		workflowInfo, err := service.GetWorkflowLabels(accessRequest)
		if err != nil {
			r.log.Warn("failed to get workflow labels from access request", "error", err, "access_request_id", accessRequest.GetName())
			continue
		}

		accessRequestIndex[workflowInfo.WorkflowRunID] = accessRequest
	}
	return accessRequestIndex
}
