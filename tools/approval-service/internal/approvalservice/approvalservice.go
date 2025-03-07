package approvalservice

import (
	"context"
	"log"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"

	"golang.org/x/sync/errgroup"
)

type ApprovalService struct {
	processor    ApprovalProcessor
	eventSources []EventSource
}

func NewApprovalService(cfg Config) (*ApprovalService, error) {
	slog.Default().Info("Starting approval service")
	tele, err := newTeleportClientFromConfig(context.Background(), cfg.Teleport)
	if err != nil {
		return nil, err
	}
	var processor ApprovalProcessor = &TeleportApprovalProcessor{
		teleportClient: tele,
	}
	accessPlugin, err := accessrequest.NewPlugin(tele, processor)
	if err != nil {
		return nil, err
	}
	return &ApprovalService{
		processor: processor,
		eventSources: []EventSource{
			githubevents.NewSource(cfg.GitHubEvents, processor),
			accessPlugin,
		},
	}, nil
}

// Run starts the approval service.
func (s *ApprovalService) Run(ctx context.Context) error {
	// 1. Setup approval processor
	_ = s.processor.Setup() // Error handling TODO

	for _, eventSource := range s.eventSources {
		_ = eventSource.Setup()
	}

	// 3. Start event sources
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

type ApprovalProcessor interface {
	// This should do things like setup API clients, as well as anything
	// needed to approve/deny events.
	Setup() error

	githubevents.DeploymentReviewEventProcessor
	accessrequest.ReviewHandler
}

type EventSource interface {
	// This should do thinks like setup API clients and webhooks.
	Setup() error

	// Handle actual requests. This should not block.
	Run(ctx context.Context) error
}
