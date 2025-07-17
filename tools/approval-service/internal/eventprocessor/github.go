package eventprocessor

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// GitHubConsumer processes GitHub events and manages the state of Access Requests for them.
type GitHubConsumer struct {
	// Identifiers for the GitHub source.
	Org  string
	Repo string

	// TeleportRole is the role that will requested for the Access Request.
	TeleportRole string

	// AccessRequestProcessor handles deployment review events and responds to changes in state of Access Requests.
	// It is not responsible for creating or managing the Access Requests themselves.
	AccessRequestProcessor GitHubAccessRequestProcessor
}

// GitHubAccessRequestProcessor is an interface that defines the methods for processing GitHub events related to Access Requests.
type GitHubAccessRequestProcessor interface {
	FindExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error)
	CreateNewAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent, req types.AccessRequest) (types.AccessRequest, error)
	accessrequest.ReviewHandler
}

// ProcessDeploymentReviewEvent processes a GitHub deployment review event.
func (p *Dispatcher) ProcessDeploymentReviewEvent(ctx context.Context, e githubevents.DeploymentReviewEvent) error {
	p.deployReviewEventC <- e
	return nil
}

// githubEventListener handles asynchronous processing of GitHub events.
// Most GitHub events should be processed asynchronously from the main request due to the potential for long-running operations
// (e.g. acquiring lease, Access Request creation, etc.).
//
// This will block until the context is done or an error occurs and is intended to be run in a goroutine.
func (p *Dispatcher) githubEventListener(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			p.log.Info("stopping GitHub event listener")
			return ctx.Err()
		case e := <-p.deployReviewEventC:
			go p.processDeploymentReviewEvent(ctx, e)
		}
	}
}

// processDeploymentReviewEvent is the consumer for GitHub deployment review events.
// It processes the event by leasing the GitHub workflow, checking for existing Access Requests, and creating new ones if necessary.
func (p *Dispatcher) processDeploymentReviewEvent(ctx context.Context, e githubevents.DeploymentReviewEvent) error {
	// Attempt to lease the GitHub workflow for the event.
	// This is used to prevent multiple processes from handling the same event at the same time which would result in
	// multiple Access Requests being created for the same event.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := p.coordinator.LeaseGitHubWorkflow(ctx, e.Organization, e.Repository, e.WorkflowID)
	if err != nil {
		return fmt.Errorf("leasing GitHub workflow: %w", err)
	}

	p.log.Info("processing GitHub deployment review event", "event", e)

	id := githubID(e.Organization, e.Repository)
	sp, ok := p.githubConsumers[id]
	if !ok {
		return fmt.Errorf("no GitHub processor for %s/%s", e.Organization, e.Repository)
	}

	existing, err := sp.AccessRequestProcessor.FindExistingAccessRequest(ctx, e)
	if err != nil {
		return fmt.Errorf("finding existing access request: %w", err)
	}

	if existing != nil {
		return sp.AccessRequestProcessor.HandleReview(ctx, existing)
	}

	// Generate the Access Request for the event.
	newReq, err := types.NewAccessRequest(uuid.NewString(), p.teleportUser, sp.TeleportRole)
	newReq.SetExpiry(time.Now().Add(p.requestTTLHours))
	if err != nil {
		return fmt.Errorf("generating access request: %w", err)
	}
	if err := p.store.StoreProcID(ctx, newReq, id); err != nil {
		return fmt.Errorf("storing GitHub processor ID: %w", err)
	}

	created, err := sp.AccessRequestProcessor.CreateNewAccessRequest(ctx, e, newReq)
	if err != nil {
		return fmt.Errorf("creating new access request: %w", err)
	}

	p.log.Info("created new access request", "name", created.GetName(), "event", e)
	return err
}

func (p *Dispatcher) ProcessWorkflowDispatchEvent(ctx context.Context, e githubevents.WorkflowDispatchEvent) error {
	// This is not implemented yet.
	return fmt.Errorf("workflow_dispatch event processing is not implemented")
}

func (p *Dispatcher) handleGitHubReview(ctx context.Context, req types.AccessRequest) error {
	id, err := p.store.GetProcID(ctx, req)
	p.log.Info("labels", "labels", req.GetStaticLabels())
	if err != nil {
		return fmt.Errorf("getting GitHub processor ID: %w", err)
	}
	sp, ok := p.githubConsumers[id]
	if !ok {
		return fmt.Errorf("no GitHub processor matching ID %q for Access Request %q", id, req.GetName())
	}

	return sp.AccessRequestProcessor.HandleReview(ctx, req)
}

func githubID(org, repo string) string {
	return fmt.Sprintf("%s/%s", org, repo)
}
