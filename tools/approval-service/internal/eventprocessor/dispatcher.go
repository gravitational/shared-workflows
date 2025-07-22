package eventprocessor

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/coordination"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventprocessor/githubprocessors"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventprocessor/store"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// Dispatcher is responsible for coordinating the processing of events from different sources to their respective consumers.
type Dispatcher struct {
	store store.ProcessorService

	teleportUser    string
	requestTTLHours time.Duration

	// workflowEventsProcessor is a map of GitHub source processors.
	// The key is a string of the form "org/repo" and the value is the GitHub source processor.
	// This is used to look up the GitHub source processor for a given GitHub event or Access Request.
	// not safe for concurrent read/write
	// this is written to during init and only read concurrently during operation.
	workflowEventsProcessor map[string]githubprocessors.WorkflowEventsProcessor

	// Channels for asynchronous processing of events.
	deployReviewEventC   chan githubevents.DeploymentReviewEvent
	accessRequestReviewC chan types.AccessRequest

	// le is the leader elector for the Dispatcher.
	le coordination.LeaderElector

	log *slog.Logger
}

// Opt is a functional option for configuring the Dispatcher.
type Opt func(d *Dispatcher) error

// WithLogger sets the logger for the Dispatcher.
func WithLogger(logger *slog.Logger) Opt {
	return func(d *Dispatcher) error {
		if logger == nil {
			return errors.New("logger cannot be nil")
		}
		d.log = logger
		return nil
	}
}

// NewDispatcher creates a new Dispatcher instance.
func NewDispatcher(teleConfig config.Teleport, le coordination.LeaderElector, store store.ProcessorService, opts ...Opt) (*Dispatcher, error) {
	d := &Dispatcher{
		teleportUser:            teleConfig.User,
		workflowEventsProcessor: make(map[string]githubprocessors.WorkflowEventsProcessor),
		le:                      le,
		log:                     slog.Default(),
		// Use the configured request TTL or default to 7 days if not set.
		requestTTLHours: cmp.Or(time.Duration(teleConfig.RequestTTLHours)*time.Hour, 7*24*time.Hour),
		// Channels for asynchronous processing of events.
		// Setting buffer size to 1 to allow for non-blocking sends but still allow for some backpressure.
		// An issue is that for large buffers, we end up with a lot of events queued in memory that will be lost if the service crashes.
		// Due to the asynchronous nature, once the event is received from the channel, it is considered "processed" and GitHub will show the event as successful.
		// Balancing throughput and backpressure is important to avoid overwhelming the system and to properly handle event processing.
		deployReviewEventC:   make(chan githubevents.DeploymentReviewEvent, 1),
		accessRequestReviewC: make(chan types.AccessRequest, 1),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return d, nil
}

// Run starts the Dispatcher and starts receiving events from the event sources.
// This handles fanning-in of events and asynchronously fanning-out to the appropriate consumers.
// It will block until the context is cancelled.
func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		select {
		case deployReviewEvent := <-d.deployReviewEventC:
			go d.onDeploymentReviewEventReceived(ctx, deployReviewEvent)
		case accessRequestReview := <-d.accessRequestReviewC:
			go d.onAccessRequestReviewed(ctx, accessRequestReview)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// HandleDeploymentReviewEventReceived multiplexes deployment review events and handles them asynchronously.
// This function will return a nil error since the processing is done asynchronously.
func (d *Dispatcher) HandleDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) error {
	d.deployReviewEventC <- e
	return nil
}

// HandleWorkflowDispatchEventReceived is a placeholder for processing workflow dispatch events.
func (d *Dispatcher) HandleWorkflowDispatchEventReceived(ctx context.Context, e githubevents.WorkflowDispatchEvent) error {
	// This is not implemented yet.
	return fmt.Errorf("workflow_dispatch event processing is not implemented")
}

// HandleAccessRequestReviewed will handle updates to the state of a Teleport Access Request.
func (d *Dispatcher) HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error {
	d.accessRequestReviewC <- req
	return nil
}

func (d *Dispatcher) onDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := d.le.LeaseHandleDeploymentReviewEventReceived(ctx, e.Organization, e.Repository, e.WorkflowID); err != nil {
		d.log.Error("Error leasing deployment review event", "event", e, "error", err)
		return
	}

	id := githubID(e.Organization, e.Repository)
	workflowEventsProcessor, ok := d.workflowEventsProcessor[id]
	if !ok {
		d.log.Error("No GitHub processor found for deployment review event", "event", e, "processor_id", id)
		return
	}

	d.log.Info("Processing deployment review event", "event", e)
	existingRequest, err := workflowEventsProcessor.FindExistingAccessRequest(ctx, e)
	if err != nil {
		d.log.Error("Error finding existing access request", "event", e, "error", err)
		return
	}

	if existingRequest != nil {
		if existingRequest.GetState() == types.RequestState_PENDING {
			// Existing request is pending, no further action needed.
			d.log.Info("Found existing pending access request", "access_request_name", existingRequest.GetName(), "event", e)
			return
		}

		// Use async processing for the access request
		d.HandleAccessRequestReviewed(ctx, existingRequest)
		return
	}

	role, err := workflowEventsProcessor.TeleportRoleForEnvironment(e.Environment)
	if err != nil {
		d.log.Error("Error getting Teleport role for environment", "environment", e.Environment, "error", err)
		return
	}

	newReq, err := types.NewAccessRequest(uuid.NewString(), d.teleportUser, role)
	if err != nil {
		d.log.Error("Error generating new access request", "event", e, "error", err)
		return
	}
	newReq.SetExpiry(time.Now().Add(d.requestTTLHours))
	if err := d.store.StoreProcID(ctx, newReq, id); err != nil {
		d.log.Error("Error storing GitHub processor ID", "event", e, "error", err)
		return
	}

	d.log.Info("Creating new access request", "name", newReq.GetName(), "event", e)
	created, err := workflowEventsProcessor.CreateAccessRequest(ctx, e, newReq)
	if err != nil {
		d.log.Error("Error creating new access request", "event", e, "error", err)
		return
	}

	d.log.Info("Created new access request", "access_request_name", created.GetName(), "event", e)
}

func (d *Dispatcher) onAccessRequestReviewed(ctx context.Context, req types.AccessRequest) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := d.le.LeaseHandleAccessRequestReviewed(ctx, req.GetName()); err != nil {
		d.log.Error("Error leasing access request", "access_request_name", req.GetName(), "error", err)
		return
	}

	id, err := d.store.GetProcID(ctx, req)
	if err != nil {
		d.log.Error("Couldn't get GitHub processor ID from Access Request", "access_request_name", req.GetName(), "error", err)
		return
	}

	workflowEventsProcessor, ok := d.workflowEventsProcessor[id]
	if !ok {
		d.log.Error("No GitHub processor found for access request", "access_request_name", req.GetName(), "processor_id", id)
		return
	}

	if err := workflowEventsProcessor.HandleAccessRequestReviewed(ctx, req); err != nil {
		d.log.Error("Handling access request reviewed", "access_request_name", req.GetName(), "error", err)
		return
	}
	d.log.Info("Handled access request reviewed", "access_request_name", req.GetName(), "processor_id", id)
}

// WithGitHubWorkflowEventProcessor registers a GitHub processor for the Dispatcher by org and repo.
// This allows the Dispatcher to handle events from workflows that run in the specified GitHub repository.
func WithGitHubWorkflowEventProcessor(org, repo string, proc githubprocessors.WorkflowEventsProcessor) Opt {
	return func(d *Dispatcher) error {
		if org == "" || repo == "" {
			return fmt.Errorf("org and repo cannot be empty")
		}
		if proc == nil {
			return errors.New("processor cannot be nil")
		}
		d.workflowEventsProcessor[githubID(org, repo)] = proc
		return nil
	}
}

func githubID(org, repo string) string {
	return org + "/" + repo
}
