package githubreconcile

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// TODO: Implement the reconciler in a second pass after first code review.

// Reconciler is a service that reconciles the state of GitHub deployment protection rules with the state of Teleport access requests.
// Events are ephemeral and failures to process results in a loss of data. This serves as a redundancy for such cases.
// To recover from this, the reconciler will periodically check the state of the deployment protection rules and the state of the access requests.
// If it detects a mismatch, it will fire an event to update the state of the access request.
type Reconciler struct {
	deployReviewEventProcessor githubevents.GitHubEventProcessor
	reviewHandler              accessrequest.ReviewHandler

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

func NewReconciler(ghClient github.Client, teleClient teleClient, org, repo string, reviewHandler accessrequest.ReviewHandler, deployProcessor githubevents.GitHubEventProcessor, opts ...Opt) (*Reconciler, error) {
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
	// Reconciler is driven by the state of GitHub workflow runs
	// If there are pending workflow runs, we need to investigate the state of their corresponding Access Request.
	runs, err := r.ghClient.ListWaitingWorkflowRuns(ctx, r.org, r.repo)
	if err != nil {
		return fmt.Errorf("failed to list pending deployment protection rules: %w", err)
	}

	if len(runs) == 0 {
		// No pending deployment protection rules, nothing to reconcile.
		return nil
	}

	runToReqMap, err := r.accessRequestsMap(ctx)
	if err != nil {
		return fmt.Errorf("failed to construct access request state map: %w", err)
	}

	for _, run := range runs {
		if req, ok := runToReqMap[run.WorkflowID]; ok {
			// Access request exists for this workflow run, check its state.
			if req.GetState() == types.RequestState_PENDING {
				continue // Access request is still pending, nothing to do.
			}

			// Access request is not pending, and we have a pending workflow run.
			// This means we need to refire the event to update the GitHub deployment protection rule.
			r.log.Info("detected access request state change, refiring event", "workflow_run_id", run.WorkflowID, "workflow_name", run.Name, "org", run.Organization, "repo", run.Repository)
			r.reviewHandler.HandleReview(ctx, req)
		}

		// No access request for this workflow run, refire the event to create one.
		r.log.Info("detected missing access request, refiring event", "workflow_run_id", run.WorkflowID, "workflow_name", run.Name, "org", run.Organization, "repo", run.Repository)
		pending, err := r.ghClient.GetPendingDeployments(ctx, run.Organization, run.Repository, run.WorkflowID)
		if err != nil {
			return fmt.Errorf("failed to get pending deployments for workflow run %d: %w", run.WorkflowID, err)
		}

		for _, deployment := range pending {
			r.deployReviewEventProcessor.ProcessDeploymentReviewEvent(ctx, githubevents.DeploymentReviewEvent{
				WorkflowID:   run.WorkflowID,
				Requester:    run.Requester,
				Organization: run.Organization,
				Repository:   run.Repository,
				Environment:  deployment.Environment,
			})
		}
	}

	return nil
}

// accessRequestsMap returns a of access request state mapped to their workflow run IDs.
func (r *Reconciler) accessRequestsMap(ctx context.Context) (map[int64]types.AccessRequest, error) {
	reqs, err := r.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{
		// TODO: Figure out filtering for Access Requests
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get access requests: %w", err)
	}

	stateMap := map[int64]types.AccessRequest{}
	for _, req := range reqs {
		if _, ok := req.GetStaticLabels()["workflow_run_id"]; !ok {
			continue
		}

		id, err := strconv.Atoi(req.GetStaticLabels()["workflow_run_id"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse workflow run ID from access request labels: %w", err)
		}
		stateMap[int64(id)] = req
	}
	return stateMap, nil
}
