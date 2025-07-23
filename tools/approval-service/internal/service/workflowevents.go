package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/store"
	teleportclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// ReleaseService processes GitHub Events and Teleport Access Request state changes that are related to GitHub workflows.
// At a high level, it waits for a GitHub Workflow to request access to an environment (signaled by deployment_protection_rule),
// and then creates a Teleport Access Request for the workflow.
// It then waits for the Access Request to be reviewed, and based on the review, it approves or rejects the workflow in GitHub.
type ReleaseService struct {
	// Required for creating new Access Requests.
	teleportUser    string
	requestTTLHours time.Duration
	teleClient      *teleportclient.Client

	ghApprover *gitHubWorkflowApprover

	// Channels for asynchronous processing of events.
	deploymentReviewEventChan chan githubevents.DeploymentReviewEvent
	accessRequestReviewChan   chan types.AccessRequest

	// For deduplication logic to ensure that we do not process similar events concurrently.
	// For example, we can receive 10s-100s of deployment review events in a short period of time for the same workflow.
	mu                  sync.Mutex
	currentlyProcessing map[string]struct{} // tracks currently processing events by their IDs

	log *slog.Logger
}

// ReleaseServiceOpts is a functional option for configuring the WorkflowEventsProcessor.
type ReleaseServiceOpts func(d *ReleaseService) error

// NewReleaseService creates a new WorkflowEventsProcessor instance.
func NewReleaseService(cfg config.Root, teleClient *teleportclient.Client, opts ...ReleaseServiceOpts) (*ReleaseService, error) {
	approver, err := newGitHubWorkflowApprover(context.Background(), cfg.EventSources.GitHub, slog.Default())
	if err != nil {
		return nil, fmt.Errorf("creating GitHub workflow approver: %w", err)
	}

	d := &ReleaseService{
		ghApprover:          approver,
		teleClient:          teleClient,
		requestTTLHours:     cmp.Or(time.Duration(cfg.ApprovalService.Teleport.RequestTTLHours)*time.Hour, 7*24*time.Hour),
		teleportUser:        cfg.ApprovalService.Teleport.User,
		currentlyProcessing: make(map[string]struct{}),
		log:                 slog.Default(),
		// Channels for asynchronous processing of events.
		// Setting buffer size to 1 to allow for non-blocking sends but still allow for some backpressure.
		// An issue is that for large buffers, we end up with a lot of events queued in memory that will be lost if the service crashes.
		// Due to the asynchronous nature, once the event is received from the channel, it is considered "processed" and GitHub will show the event as successful.
		// Balancing throughput and backpressure is important to avoid overwhelming the system and to properly handle event processing.
		deploymentReviewEventChan: make(chan githubevents.DeploymentReviewEvent, 1),
		accessRequestReviewChan:   make(chan types.AccessRequest, 1),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return d, nil
}

// Run starts the WorkflowEventsProcessor and begins listening for events.
// This method fans-in events from multiple sources and asynchronously fans-out to the appropriate processors.
// It blocks until the context is cancelled.
func (w *ReleaseService) Run(ctx context.Context) error {
	for {
		select {
		case deployReviewEvent := <-w.deploymentReviewEventChan:
			go w.onDeploymentReviewEventReceived(ctx, deployReviewEvent)
		case accessRequestReview := <-w.accessRequestReviewChan:
			go w.onAccessRequestReviewed(ctx, accessRequestReview)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// HandleDeploymentReviewEventReceived multiplexes deployment review events and handles them asynchronously.
// This function will return a nil error since the processing is done asynchronously.
func (w *ReleaseService) HandleDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) error {
	w.deploymentReviewEventChan <- e
	return nil
}

// HandleWorkflowDispatchEventReceived is a placeholder for processing workflow dispatch events.
func (w *ReleaseService) HandleWorkflowDispatchEventReceived(ctx context.Context, e githubevents.WorkflowDispatchEvent) error {
	// This is not implemented yet.
	return fmt.Errorf("workflow_dispatch event processing is not implemented")
}

// HandleAccessRequestReviewed will handle updates to the state of a Teleport Access Request.
func (w *ReleaseService) HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error {
	w.accessRequestReviewChan <- req
	return nil
}

func (w *ReleaseService) onDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) {
	// One Access Request should be created per workflow run and environment.
	eventID := fmt.Sprintf("%s/%s/%d/%s", e.Organization, e.Repository, e.WorkflowID, e.Environment)
	if !w.tryStartEventProcessing(eventID) {
		// Already processing this event, skip it.
		w.log.Debug("Skipping already processed event", "event_id", eventID)
		return
	}
	defer w.finishEventProcessing(eventID)

	w.log.Info("Processing deployment review event", "event", e)
	existingRequest, err := w.findExistingAccessRequest(ctx, e)
	if err != nil {
		w.log.Error("Attempting to find existing access request", "error", err)
		return
	}

	if existingRequest != nil {
		// An access request already exists for this workflow run.
		// Check its state to determine the next steps.

		if existingRequest.GetState() == types.RequestState_PENDING {
			// Existing request is pending, no further action needed.
			w.log.Info("Found existing pending access request", "access_request_name", existingRequest.GetName(), "event", e)
			return
		}

		// Existing request is already reviewed (approved/rejected).
		// This happens when a previous job in the workflow has already triggered the review process.
		_ = w.HandleAccessRequestReviewed(ctx, existingRequest) // Async handling does not return an error.
		return
	}

	req, err := w.createAccessRequest(ctx, e)
	if err != nil {
		w.log.Error("Error creating new access request", "event", e, "error", err)
		return
	}

	w.log.Info("Created new access request", "access_request_name", req.GetName(), "event", e)
}

// findExistingAccessRequest checks if an Access Request already exists for the given GitHub deployment review event.
// It returns the Access Request if it exists, or nil if it does not.
// An error indicates a problem with the Teleport API, not that an Access Request does not exist.
//
// Three main things can determined from this:
//  1. If no Access Request exists, we need to create one.
//  2. If an Access Request exists, and is pending, no further action is needed.
//  3. If an Access Request exists, and is not pending, we can update the state of the GitHub deployment accordingly.
func (w *ReleaseService) findExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
	list, err := w.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{})
	if err != nil {
		return nil, fmt.Errorf("getting access requests: %w", err)
	}

	for _, req := range list {
		info, err := store.GetWorkflowInfoFromLabels(ctx, req)
		if err != nil {
			w.log.Debug("failed to get workflow info for access request", "access_request_name", req.GetName(), "error", err)
			// Not all Access Requests will have workflow info, so we can ignore this error.
			continue
		}

		// Check if the Access Request matches the GitHub deployment review event.
		if info.MatchesEvent(e) {
			w.log.Info("Found existing access request for deployment review event", "access_request_name", req.GetName(), "event", e)
			return req, nil
		}
	}

	// No existing access request found.
	return nil, nil
}

// createAccessRequest creates a new Access Request for the given GitHub deployment review event.
func (w *ReleaseService) createAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
	role, err := w.ghApprover.teleportRoleForEnvironment(e.Environment)
	if err != nil {
		return nil, fmt.Errorf("getting Teleport role for environment %q: %w", e.Environment, err)
	}
	newReq, err := types.NewAccessRequest(uuid.NewString(), w.teleportUser, role)
	if err != nil {
		return nil, fmt.Errorf("generating new access request: %w", err)
	}
	newReq.SetExpiry(time.Now().Add(w.requestTTLHours))
	err = store.SetWorkflowInfoLabels(ctx, newReq, store.GitHubWorkflowInfo{
		Org:           e.Organization,
		Repo:          e.Repository,
		Env:           e.Environment,
		WorkflowRunID: e.WorkflowID,
	})
	if err != nil {
		return nil, fmt.Errorf("storing workflow info: %w", err)
	}

	reason, err := w.ghApprover.generateAccessRequestReason(e.WorkflowID, e.Environment)
	if err != nil {
		return nil, fmt.Errorf("generating access request reason: %w", err)
	}
	newReq.SetRequestReason(reason)

	created, err := w.teleClient.CreateAccessRequestV2(ctx, newReq)
	if err != nil {
		return nil, fmt.Errorf("creating access request: %w", err)
	}

	return created, nil
}

// tryStartEventProcessing attempts to start processing an event if it's not already being processed.
// Returns true if processing should proceed, false if the event is already being processed.
// This method provides deduplication to prevent concurrent processing of the same workflow event.
func (w *ReleaseService) tryStartEventProcessing(eventID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.currentlyProcessing[eventID]; exists {
		// Already processing this event, skip it.
		return false
	}

	// Mark this event as currently being processed.
	w.currentlyProcessing[eventID] = struct{}{}
	return true
}

// finishEventProcessing marks an event as finished processing, allowing it to be processed again in the future.
// This removes the event from the currently processing set to prevent memory leaks.
func (w *ReleaseService) finishEventProcessing(eventID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.currentlyProcessing, eventID)
}

// onAccessRequestReviewed processes the Access Request review event.
func (w *ReleaseService) onAccessRequestReviewed(ctx context.Context, req types.AccessRequest) {
	info, err := store.GetWorkflowInfoFromLabels(ctx, req)
	if err != nil {
		// If we cannot find the workflow info, we cannot process the request.
		// This is likely due to the access request not having the required labels.
		w.log.Debug("Getting workflow info for access request", "access_request_name", req.GetName(), "error", err)
		return
	}

	if err := w.ghApprover.handleDecisionForAccessRequestReviewed(ctx, req.GetState(), info.Env, info.WorkflowRunID); err != nil {
		w.log.Error("Error handling access request reviewed", "access_request_name", req.GetName(), "error", err)
		return
	}
	w.log.Info("Handled access request reviewed", "access_request_name", req.GetName(), "org", info.Org, "repo", info.Repo)
}

// WithLogger sets the logger for the WorkflowEventsProcessor.
func WithLogger(logger *slog.Logger) ReleaseServiceOpts {
	return func(d *ReleaseService) error {
		if logger == nil {
			return errors.New("logger cannot be nil")
		}
		d.log = logger
		return nil
	}
}
