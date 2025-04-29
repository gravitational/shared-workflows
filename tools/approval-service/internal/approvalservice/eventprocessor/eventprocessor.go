package eventprocessor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/coordination"
	"github.com/gravitational/teleport/api/types"
)

// Processor ties together Access Requests and service specific sources and processors.
// It is directly responsible for all CRUD operations on Access Requests.
// It is also responsible of informing processors of changes in state of Access Requests and sources.
type Processor struct {
	teleportUser string
	sp           *SourceProcessors

	// githubProcessors is a map of GitHub source processors.
	// The key is a string of the form "org/repo" and the value is the GitHub source processor.
	// This is used to look up the GitHub source processor for a given GitHub event or Access Request.
	// not safe for concurrent read/write
	// this is written to during init and only read concurrently during operation.
	githubProcessors map[string]*GitHubSourceProcessor

	coordinator Coordinator

	log *slog.Logger
}

type Coordinator interface {
	LeaseAccessRequest(ctx context.Context, id string) error
	LeaseGitHubWorkflow(ctx context.Context, org, repo string, workflowID int64) error
}

type SourceProcessors struct {
	GitHub []*GitHubSourceProcessor
}

func New(ctx context.Context, teleConfig config.Teleport, sp *SourceProcessors, coordinator Coordinator) (*Processor, error) {
	return &Processor{
		teleportUser:     teleConfig.User,
		sp:               sp,
		githubProcessors: make(map[string]*GitHubSourceProcessor),
		coordinator:      coordinator,
		log:              slog.Default(),
	}, nil
}

func (p *Processor) Setup(ctx context.Context) error {
	for _, gh := range p.sp.GitHub {
		p.log.Info("Registering GitHub source processor", "id", githubID(gh.Org, gh.Repo))
		p.githubProcessors[githubID(gh.Org, gh.Repo)] = gh
	}
	return nil
}

// HandleReview will handle updates to the state of a Teleport Access Request.
func (p *Processor) HandleReview(ctx context.Context, req types.AccessRequest) error {
	ctx, cancal := context.WithCancel(ctx)
	defer cancal()

	err := p.coordinator.LeaseAccessRequest(ctx, req.GetName())
	switch {
	case errors.Is(err, coordination.ErrAlreadyLeased):
		p.log.Debug("Access request already leased", "name", req.GetName())
		return nil
	case err != nil:
		return fmt.Errorf("leasing access request: %w", err)
	}

	// Currently only GitHub is supported.
	// If another source is added, will need to be updated to serialize the processor type to delegate to.
	return p.handleGitHubReview(ctx, req)
}
