package approvalservice

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/coordination"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/eventprocessor"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/eventprocessor/githubprocessor"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	teleportClient "github.com/gravitational/teleport/api/client"

	"golang.org/x/sync/errgroup"
)

type ApprovalService struct {
	cfg config.Root

	// eventSources is a list of event sources that the approval service listens to.
	// This will be things like GitHub webhook events, Teleport access request updates, etc.
	eventSources []EventSource

	// processor is the event processor for the approval service.
	processor EventProcessor

	log *slog.Logger
	ctx context.Context
}

type EventSource interface {
	// This should do thinks like setup API clients and webhooks.
	Setup(ctx context.Context) error

	// Handle actual requests. This should not block.
	Run(ctx context.Context) error
}

// EventProcessor provides methods for processing events fromm our event sources.
// This will be passed to the event sources to handle certain actions provided by the event source.
type EventProcessor interface {
	Setup(ctx context.Context) error
	githubevents.DeploymentReviewEventProcessor
	accessrequest.ReviewHandler
}

// Opt is an option for the approval service.
type Opt func(*ApprovalService) error

// WithLogger sets the logger for the approval service.
func WithLogger(logger *slog.Logger) Opt {
	return func(s *ApprovalService) error {
		s.log = logger
		return nil
	}
}

// WithContext sets the context for the approval service.
// This is primarily used for Teleport client Dialing.
func WithContext(ctx context.Context) Opt {
	return func(s *ApprovalService) error {
		s.ctx = ctx
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
	WithContext(context.Background()),
}

// NewApprovalService initializes a new approval service from config.
// An error is returned if the service cannot be initialized e.g. if the Teleport client cannot connect.
func NewApprovalService(cfg config.Root, opts ...Opt) (*ApprovalService, error) {
	a, err := newWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	if len(cfg.EventSources.GitHub) == 0 {
		// Only github event sources are supported for now.
		return nil, fmt.Errorf("no event sources configured, refusing to start")
	}

	a.log.Info("Initializing approval service")
	// Teleport client is common to event source and processor
	tele, err := newTeleportClientFromConfig(a.ctx, cfg.ApprovalService.Teleport)
	if err != nil {
		return nil, err
	}

	a.eventSources = []EventSource{}
	sp := &eventprocessor.SourceProcessors{
		GitHub: []*eventprocessor.GitHubSourceProcessor{},
	}

	a.log.Info("Initializing coordinator")
	coord, err := coordination.NewCoordinator(
		coordination.WithLogger(a.log),
		coordination.GitHubWorkflowLeaseDuration(1*time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("creating coordinator: %w", err)
	}

	// Initialize event processor
	a.log.Info("Initializing event processor")
	var processor EventProcessor
	processor, err = eventprocessor.New(a.ctx, cfg.ApprovalService.Teleport, sp, coord)
	if err != nil {
		return nil, fmt.Errorf("initializing event processor: %w", err)
	}
	a.processor = processor

	// Initialize GitHub event sources
	a.log.Info("Initializing GitHub event sources")
	for _, gh := range cfg.EventSources.GitHub {
		a.log.Info("Initializing GitHub event source", "org", gh.Org, "repo", gh.Repo)
		ghSource := githubevents.NewSource(gh, a.processor, githubevents.WithLogger(a.log))
		a.eventSources = append(a.eventSources, ghSource)

		ghProcessor, err := githubprocessor.New(a.ctx, gh, tele, githubprocessor.WithLogger(a.log))
		if err != nil {
			return nil, fmt.Errorf("creating GitHub processor: %w", err)
		}
		sp.GitHub = append(sp.GitHub, &eventprocessor.GitHubSourceProcessor{
			AccessRequestProcessor: ghProcessor,
			Org:                    gh.Org,
			Repo:                   gh.Repo,
			TeleportRole:           cfg.ApprovalService.Teleport.RoleToRequest,
		})
	}

	// Initialize AccessRequest plugin that sources events from Teleport
	a.log.Info("Initializing access request plugin")
	accessPlugin, err := accessrequest.NewPlugin(tele, processor)
	if err != nil {
		return nil, err
	}
	a.eventSources = append(a.eventSources, accessPlugin)

	return a, nil
}

func newWithOpts(opts ...Opt) (*ApprovalService, error) {
	s := &ApprovalService{}
	for _, opt := range append(defaultOpts, opts...) {
		if err := opt(s); err != nil {
			return nil, fmt.Errorf("error applying option: %w", err)
		}
	}

	return s, nil
}

func (a *ApprovalService) Setup(ctx context.Context) error {
	for _, eventSource := range a.eventSources {
		if err := eventSource.Setup(ctx); err != nil {
			return fmt.Errorf("setting up event source: %w", err)
		}
	}

	if err := a.processor.Setup(ctx); err != nil {
		return fmt.Errorf("setting up event processor: %w", err)
	}
	a.log.Info("Approval service setup complete")
	return nil
}

// Run starts the approval service.
func (a *ApprovalService) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, eventSource := range a.eventSources {
		eg.Go(func() error {
			return eventSource.Run(ctx)
		})
	}

	eg.Go(func() error {
		return a.runHealthEndpoint(ctx)
	})

	slog.Default().Info("Approval service started")
	// Block until an event source has a fatal error
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("approval service encountered fatal error: %w", err)
	}
	return nil
}

func newTeleportClientFromConfig(ctx context.Context, cfg config.Teleport) (*teleportClient.Client, error) {
	slog.Default().Info("Initializing Teleport client")
	client, err := teleportClient.New(ctx, teleportClient.Config{
		Addrs: cfg.ProxyAddrs,
		Credentials: []teleportClient.Credentials{
			teleportClient.LoadIdentityFile(cfg.IdentityFile),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initializing teleport client: %w", err)
	}

	return client, nil
}
