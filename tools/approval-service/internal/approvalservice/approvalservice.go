package approvalservice

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	teleportClient "github.com/gravitational/teleport/api/client"

	"golang.org/x/sync/errgroup"
)

type ApprovalService struct {
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
	Setup() error

	// Handle actual requests. This should not block.
	Run(ctx context.Context) error
}

// EventProcessor provides methods for processing events fromm our event sources.
// This will be passed to the event sources to handle certain actions provided by the event source.
type EventProcessor interface {
	// This should do things like setup API clients, as well as anything
	// needed to approve/deny events.
	Setup() error

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
func NewApprovalService(cfg Config, opts ...Opt) (*ApprovalService, error) {
	a, err := newWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	a.log.Info("Initializing approval service")
	// Teleport client is common to event source and processor
	tele, err := newTeleportClientFromConfig(a.ctx, cfg.Teleport)
	if err != nil {
		return nil, err
	}

	// Initialize GitHub client
	a.log.Info("Initializing GitHub client")
	githubClient, err := newGitHubClientFromConfig(cfg.GitHubApp)
	if err != nil {
		return nil, err
	}

	// Initialize event processor
	var processor EventProcessor = &processor{
		TeleportUser:   cfg.Teleport.User,
		TeleportRole:   cfg.Teleport.RoleToRequest,
		teleportClient: tele,
		githubClient:   githubClient,
	}
	a.processor = processor

	// Initialize event sources
	accessPlugin, err := accessrequest.NewPlugin(tele, processor)
	if err != nil {
		return nil, err
	}
	a.eventSources = []EventSource{
		githubevents.NewSource(cfg.GitHubEvents, processor),
		accessPlugin,
	}

	if err := a.processor.Setup(); err != nil {
		return nil, fmt.Errorf("setting up approval processor: %w", err)
	}

	for _, eventSource := range a.eventSources {
		if err := eventSource.Setup(); err != nil {
			return nil, fmt.Errorf("setting up event source: %w", err)
		}
	}

	return a, nil
}

func newWithOpts(opts ...Opt) (*ApprovalService, error) {
	a := &ApprovalService{}
	for _, opt := range defaultOpts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("error applying default option: %w", err)
		}
	}

	for _, opt := range opts {
		if err := opt(a); err != nil {
			return nil, fmt.Errorf("error applying option: %w", err)
		}
	}
	return a, nil
}

// Run starts the approval service.
func (s *ApprovalService) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, eventSource := range s.eventSources {
		eg.Go(func() error {
			return eventSource.Run(ctx)
		})
	}

	slog.Default().Info("Approval service started")
	// Block until an event source has a fatal error
	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}
	return nil
}

func newTeleportClientFromConfig(ctx context.Context, cfg TeleportConfig) (*teleportClient.Client, error) {
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

func newGitHubClientFromConfig(cfg GitHubAppConfig) (client *github.Client, err error) {
	f, err := os.Open(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("opening private key: %w", err)
	}
	defer func() {
		closeErr := f.Close()
		if closeErr != nil && err == nil { // Only propagate error closing if NO other error occurred
			err = fmt.Errorf("closing private key: %w", closeErr)
		}
	}()
	pKey, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	client, err = github.NewForApp(cfg.AppID, cfg.InstallationID, pKey)
	if err != nil {
		return nil, fmt.Errorf("initializing GitHub client: %w", err)
	}

	return client, nil
}
