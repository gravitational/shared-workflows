package approvalservice

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	teleportClient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

type processor struct {
	// TeleportUser is the user that the approval service will use to request access.
	TeleportUser string
	// TeleportRole is the role that the approval service will request access to.
	TeleportRole string

	teleportClient teleClient
	githubClient   ghClient
	log            *slog.Logger
}

func (p *processor) Setup() error {
	// Setup Teleport API client
	// TODO: Get environment IDs from string names
	return nil
}

func (p *processor) ProcessDeploymentReviewEvent(e githubevents.DeploymentReviewEvent, valid bool) error {
	// Create access request
	// The name of an access request must be a UUID
	name := uuid.New().String()
	p.log.Info("Creating access request", "access_request_name", name)
	req, err := types.NewAccessRequest(name, p.TeleportUser, p.TeleportRole)
	if err != nil {
		return fmt.Errorf("generating access request: %w", err)
	}

	req.SetRequestReason("Deployment review for environment " + e.Environment)
	req.SetStaticLabels(map[string]string{
		"workflow_run_id": strconv.Itoa(int(e.WorkflowID)),
		"organization":    e.Organization,
		"repository":      e.Repository,
	})
	created, err := p.teleportClient.CreateAccessRequestV2(context.TODO(), req)
	if err != nil {
		return fmt.Errorf("creating access request %q: %w", name, err)
	}

	if err != nil {
		return fmt.Errorf("creating access request %q: %w", name, err)
	}
	p.log.Info("Created access request", "access_request_name", created.GetName())
	return nil
}

func (p *processor) HandleReview(ctx context.Context, req types.AccessRequest) error {
	p.log.Info("Handling review", "access_request_name", req.GetName())

	runIDLabel := req.GetStaticLabels()["workflow_run_id"]
	if runIDLabel == "" {
		return fmt.Errorf("missing workflow run ID in access request %s", req.GetName())
	}

	orgLabel := req.GetStaticLabels()["organization"]
	if orgLabel == "" {
		return fmt.Errorf("missing organization in access request %s", req.GetName())
	}

	repoLabel := req.GetStaticLabels()["repository"]
	if repoLabel == "" {
		return fmt.Errorf("missing repository in access request %s", req.GetName())
	}

	runID, err := strconv.Atoi(runIDLabel)
	if err != nil {
		return fmt.Errorf("parsing workflow run ID: %w", err)
	}

	state := github.PendingDeploymentApprovalStateApproved
	if req.GetState() == types.RequestState_DENIED {
		state = github.PendingDeploymentApprovalStateRejected
	}

	p.githubClient.UpdatePendingDeployment(ctx, github.PendingDeploymentInfo{
		Org:     orgLabel,
		Repo:    repoLabel,
		RunID:   int64(runID),
		State:   state,
		EnvIDs:  []int64{},
		Comment: "",
	})
	return nil
}

// Small interfaces to make testing easier
type teleClient interface {
	CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error)
}
type ghClient interface {
	UpdatePendingDeployment(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error)
}

var _ teleClient = &teleportClient.Client{}
var _ ghClient = &github.Client{}
