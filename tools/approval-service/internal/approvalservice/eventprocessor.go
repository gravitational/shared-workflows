package approvalservice

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	teleportClient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

type processor struct {
	teleportClient *teleportClient.Client
	githubClient   *github.Client
}

func (p *processor) Setup() error {
	// Setup Teleport API client
	return nil
}

func (p *processor) ProcessDeploymentReviewEvent(e githubevents.DeploymentReviewEvent, valid bool) error {
	// 1. Create a new role:
	// 	* Set TTL to value in RFD
	// 	* Encode event information in role for recordkeeping
	slog.Default().Info("Processing deployment review", "event", e)
	p.teleportClient.CreateRole(context.Background(), &types.RoleV6{})

	// 2. Request access to the role. Include the same info as the role,
	//    for reviewer visibility.
	req, err := p.createAccessRequest(context.Background(), createAccessRequestOpts{
		User:        "bot-approval-service",
		Description: fmt.Sprintf("Requesting access to the %s environment", e.Environment),
		Roles:       []string{"gha-build-prod"},
	})

	if err != nil {
		return fmt.Errorf("creating access request: %w", err)
	}
	slog.Default().Info("Created access request", "request_id", req.GetName())

	// 3. Wait for the request to be approved or denied.
	// This may block for a long time (minutes, hours, days).
	// Timeout if it takes too long.
	return nil
}

type createAccessRequestOpts struct {
	// User is the user requesting access.
	User string
	// Description is the description of the access request.
	Description string
	// Roles are the roles the user is requesting.
	Roles []string
}

// CreateAccessRequest creates an access request for the approval service.
func (p *processor) createAccessRequest(ctx context.Context, opts createAccessRequestOpts) (types.AccessRequest, error) {
	slog.Default().Info("Creating access request", "user", opts.User, "roles", opts.Roles)
	req, err := types.NewAccessRequest(uuid.New().String(), opts.User, opts.Roles...)
	if err != nil {
		return nil, fmt.Errorf("generating access request: %w", err)
	}

	req.SetRequestReason(opts.Description)
	created, err := p.teleportClient.CreateAccessRequestV2(ctx, req)
	if err != nil {
		slog.Error("creating access request", "error", err)
		return nil, fmt.Errorf("creating access request: %w", err)
	}
	return created, nil
}

func (p *processor) HandleApproval(ctx context.Context, event types.Event) error {
	slog.Default().Info("Handling approval", "event", event)
	return nil
}

func (p *processor) HandleRejection(ctx context.Context, event types.Event) error {
	slog.Default().Info("Handling rejection", "event", event)
	return nil
}
