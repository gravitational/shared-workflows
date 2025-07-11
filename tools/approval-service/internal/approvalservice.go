/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	teleportclient "github.com/gravitational/teleport/api/client"

	"golang.org/x/sync/errgroup"
)

// ApprovalService configured and runs the various components of the approval service.
// It sets up event sources and an event processor to handle events from those sources.
type ApprovalService struct {
	// eventSources is a list of event sources that the approval service listens to.
	// This will be things like GitHub webhook events, Teleport access request updates, etc.
	eventSources []EventSource

	// processor is the event processor for the approval service.
	processor EventProcessor

	log *slog.Logger
}

// EvenrSource provides methods for setting up and running event sources.
// This is an interface that allows us to abstract different event sources like GitHub webhooks, Teleport access requests, etc.
// Each event source should implement this interface to provide its own setup and run logic.
type EventSource interface {
	// This should do thinks like setup API clients and webhooks.
	Setup(ctx context.Context) error

	// Handle actual requests. This should not block.
	Run(ctx context.Context) error
}

// EventProcessor provides methods for processing events from our event sources.
// This will be passed to the event sources to handle certain actions provided by the event source.
type EventProcessor interface {
	Setup(ctx context.Context) error
	githubevents.GitHubEventProcessor
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

// NewApprovalService initializes a new approval service from config.
// An error is returned if the service cannot be initialized e.g. if the Teleport client cannot connect.
func NewApprovalService(ctx context.Context, cfg config.Root, opts ...Opt) (*ApprovalService, error) {
	a := &ApprovalService{
		log: slog.Default(),
	}

	// Apply options to the approval service.
	for _, o := range opts {
		if err := o(a); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}
	a.log.Info("Initializing approval service")

	// Teleport client is common to event source and processor
	tele, err := newTeleportClientFromConfig(ctx, cfg.ApprovalService.Teleport)
	if err != nil {
		return nil, fmt.Errorf("creating new teleport client from config: %w", err)
	}

	// Initialize server that listens for webhook events
	srv, err := a.newServer(cfg, a.processor)
	if err != nil {
		return nil, fmt.Errorf("creating server: %w", err)
	}
	a.eventSources = append(a.eventSources, srv)

	// Initialize AccessRequest plugin that sources events from Teleport
	a.log.Info("Initializing access request plugin")
	accessPlugin, err := accessrequest.NewEventWatcher(
		tele,
		nil, // TODO: Implemented in next PR
		accessrequest.WithRequesterFilter(cfg.ApprovalService.Teleport.User),
		accessrequest.WithLogger(a.log),
	)
	if err != nil {
		return nil, fmt.Errorf("creating access request plugin: %w", err)
	}
	a.eventSources = append(a.eventSources, accessPlugin)

	return a, nil
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

	slog.Default().Info("Approval service started")
	// Block until an event source has a fatal error
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("approval service encountered fatal error: %w", err)
	}
	return nil
}

func newTeleportClientFromConfig(ctx context.Context, cfg config.Teleport) (*teleportclient.Client, error) {
	slog.Default().Info("Initializing Teleport client")
	client, err := teleportclient.New(ctx, teleportclient.Config{
		Addrs: cfg.ProxyAddrs,
		Credentials: []teleportclient.Credentials{
			teleportclient.LoadIdentityFile(cfg.IdentityFile),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initializing teleport client: %w", err)
	}

	return client, nil
}

func (a *ApprovalService) newServer(cfg config.Root, processor EventProcessor) (*eventsources.Server, error) {
	opts := []eventsources.ServerOpt{
		eventsources.WithLogger(a.log),
		eventsources.WithAddress(cfg.ApprovalService.ListenAddr),
		eventsources.WithHandler("/health", a.healthcheckHandler()),
	}

	// GitHub event sources are webhooks that are registered as handlers on the HTTP server.
	for _, gh := range cfg.EventSources.GitHub {
		gitHubSource, err := githubevents.NewSource(gh, processor)
		if err != nil {
			return nil, fmt.Errorf("creating github source: %w", err)
		}
		opts = append(opts, eventsources.WithHandler(gh.Path, gitHubSource.Handler()))
	}

	return eventsources.NewServer(opts...)
}

// This is a simple healthcheck handler that checks if the server is healthy.
func (a *ApprovalService) healthcheckHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
