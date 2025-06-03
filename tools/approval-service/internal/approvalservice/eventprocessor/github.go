package eventprocessor

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/coordination"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// GitHubSourceProcessor processes GitHub events and manages the state of Access Requests for them.
type GitHubSourceProcessor struct {
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

func (p *Processor) ProcessDeploymentReviewEvent(ctx context.Context, e githubevents.DeploymentReviewEvent, valid bool) error {
	err := p.processDeploymentReviewEvent(ctx, e, valid)
	if err != nil {
		p.log.Error("failed to process GitHub deployment review event", "error", err, "event", e)
		return err
	}
	return nil
}

func (p *Processor) processDeploymentReviewEvent(ctx context.Context, e githubevents.DeploymentReviewEvent, valid bool) error {
	// Attempt to lease the GitHub workflow for the event.
	// This is used to prevent multiple processes from handling the same event at the same time which would result in
	// multiple Access Requests being created for the same event.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := p.coordinator.LeaseGitHubWorkflow(ctx, e.Organization, e.Repository, e.WorkflowID)
	if err != nil {
		if err == coordination.ErrAlreadyLeased {
			p.log.Debug("GitHub workflow already leased", "workflowID", e.WorkflowID)
			return nil
		}
		if err == coordination.ErrInternalRateLimitExceeded {
			p.log.Debug("internal rate limit exceeded for GitHub workflow lease", "workflowID", e.WorkflowID)
			return nil
		}
		return fmt.Errorf("leasing GitHub workflow: %w", err)
	}

	p.log.Info("processing GitHub deployment review event", "event", e, "valid", valid)

	id := githubID(e.Organization, e.Repository)
	sp, ok := p.githubProcessors[id]
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
	if err != nil {
		return fmt.Errorf("generating access request: %w", err)
	}
	if err := p.storeGitHubProcessorID(id, newReq); err != nil {
		return fmt.Errorf("storing GitHub processor ID: %w", err)
	}

	created, err := sp.AccessRequestProcessor.CreateNewAccessRequest(ctx, e, newReq)
	if err != nil {
		return fmt.Errorf("creating new access request: %w", err)
	}

	p.log.Info("created new access request", "name", created.GetName(), "event", e)
	return nil
}

func (p *Processor) handleGitHubReview(ctx context.Context, req types.AccessRequest) error {
	id, err := p.getGitHubProcessorID(req)
	p.log.Info("labels", "labels", req.GetStaticLabels())
	if err != nil {
		return fmt.Errorf("getting GitHub processor ID: %w", err)
	}
	sp, ok := p.githubProcessors[id]
	if !ok {
		return fmt.Errorf("no GitHub processor matching ID %q for Access Request %q", id, req.GetName())
	}

	return sp.AccessRequestProcessor.HandleReview(ctx, req)
}

func (p *Processor) storeGitHubProcessorID(id string, req types.AccessRequest) error {
	labels := req.GetStaticLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["procid"] = id
	req.SetStaticLabels(labels)
	return nil
}

func (p *Processor) getGitHubProcessorID(req types.AccessRequest) (string, error) {
	labels := req.GetStaticLabels()
	if labels == nil {
		return "", fmt.Errorf("no labels found for access request %s", req.GetName())
	}
	id, ok := labels["procid"]
	if !ok {
		return "", fmt.Errorf("no GitHub processor ID found for access request %s", req.GetName())
	}
	return id, nil
}

func githubID(org, repo string) string {
	return fmt.Sprintf("%s/%s", org, repo)
}
