package approvalservice

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
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

	// envNameToID is a cache of environment names to their IDs.
	// This is expected to be read-only after Setup() is called.
	// If this is not the case, it will need to be protected by a mutex.
	envNameToID map[string]int64
	// validation is used to populate envNameToID during setup.
	validation []config.Validation
}

// Small interfaces around the Teleport and GitHub clients to keep implementation details separate.
type teleClient interface {
	CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error)
}
type ghClient interface {
	UpdatePendingDeployment(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error)
	GetEnvironment(ctx context.Context, info github.GetEnvironmentInfo) (github.Environment, error)
}

var _ teleClient = &teleportClient.Client{}
var _ ghClient = &github.Client{}

func newProcessor(cfg config.Root, ghClient ghClient, teleClient teleClient) *processor {
	return &processor{
		TeleportUser: cfg.Teleport.User,
		TeleportRole: cfg.Teleport.RoleToRequest,

		teleportClient: teleClient,
		githubClient:   ghClient,
		validation:     cfg.GitHubEvents.Validation,
		envNameToID:    make(map[string]int64),
		log:            slog.Default(),
	}
}

func (p *processor) Setup() error {
	if len(p.validation) == 0 {
		return fmt.Errorf("no environments configured")
	}
	// Get environment IDs for each environment we support
	for _, v := range p.validation {
		for _, env := range v.Environments {
			env, err := p.githubClient.GetEnvironment(context.TODO(), github.GetEnvironmentInfo{
				Org:         v.Org,
				Repo:        v.Repo,
				Environment: env,
			})
			if err != nil {
				return fmt.Errorf("getting environment %q: %w", env, err)
			}
			p.envNameToID[envEntry(v.Org, v.Repo, env.Name)] = env.ID
		}
	}

	return nil
}

func (p *processor) ProcessDeploymentReviewEvent(e githubevents.DeploymentReviewEvent, valid bool) error {
	if !valid {
		// TODO: Create a rejected access request if the event is invalid (e.g. incorrect org, env, user, etc.)
		//       This should be useful for audit purposes.
		return fmt.Errorf("invalid event")
	}

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
		"environment":     e.Environment,
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

	env := req.GetStaticLabels()["environment"]
	if env == "" {
		return fmt.Errorf("missing environment in access request %s", req.GetName())
	}

	envID, ok := p.envNameToID[envEntry(orgLabel, repoLabel, env)]
	if !ok {
		return fmt.Errorf("encountered unkown environment %q, this indicates a problem during setup", envEntry(orgLabel, repoLabel, env))
	}

	runID, err := strconv.Atoi(runIDLabel)
	if err != nil {
		return fmt.Errorf("parsing workflow run ID: %w", err)
	}

	var state github.PendingDeploymentApprovalState
	switch req.GetState() {
	case types.RequestState_APPROVED:
		state = github.PendingDeploymentApprovalStateApproved
	default:
		state = github.PendingDeploymentApprovalStateRejected
	}

	p.githubClient.UpdatePendingDeployment(ctx, github.PendingDeploymentInfo{
		Org:     orgLabel,
		Repo:    repoLabel,
		RunID:   int64(runID),
		State:   state,
		EnvIDs:  []int64{envID},
		Comment: "Approved by pipeline approval service - " + req.GetName(),
	})
	return nil
}

func envEntry(org, repo, env string) string {
	return fmt.Sprintf("%s/%s/%s", org, repo, env)
}
