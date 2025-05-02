package accessrequest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// Plugin is an Access Request plugin that listens for events from Teleport.
type Plugin struct {
	teleportClient *client.Client
	reviewHandler  ReviewHandler

	log *slog.Logger
}

// ReviewHandler is an interface for handling approval and rejection events.
type ReviewHandler interface {
	HandleReview(ctx context.Context, req types.AccessRequest) error
}

// NewPlugin creates a new Access Request plugin.
func NewPlugin(client *client.Client, handler ReviewHandler, opts ...Opt) (*Plugin, error) {
	var p Plugin
	for _, opt := range append(defaultOpts, opts...) {
		if err := opt(&p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}
	p.teleportClient = client
	p.reviewHandler = handler
	return &p, nil
}

func (p *Plugin) Setup(ctx context.Context) error {
	return nil
}

type Opt func(*Plugin) error

// WithLogger sets the logger for the plugin.
func WithLogger(logger *slog.Logger) Opt {
	return func(p *Plugin) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		p.log = logger
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
}

// Run starts the plugin and listens for events.
// It will block until the context is cancelled.
func (p *Plugin) Run(ctx context.Context) error {
	watch, err := p.teleportClient.NewWatcher(ctx, types.Watch{
		Kinds: []types.WatchKind{
			// AccessRequest is the resource we are interested in.
			{Kind: types.KindAccessRequest},
		},
	})

	if err != nil {
		return err
	}
	defer watch.Close()

	p.log.Info("Starting the watcher job")

	for {
		select {
		case e := <-watch.Events():
			if err := p.handleEvent(ctx, e); err != nil {
				p.log.Error("Error handling event", "error", err)
			}
		case <-watch.Done():
			if err := watch.Error(); err != nil {
				return fmt.Errorf("watcher error: %w", err)
			}
			p.log.Info("The watcher job is finished")
			return nil
		}
	}
}

func (p *Plugin) handleEvent(ctx context.Context, event types.Event) error {
	if event.Resource == nil {
		return nil
	}

	if _, ok := event.Resource.(*types.WatchStatusV1); ok {
		p.log.Info("Successfully started listening for Access Requests...")
		return nil
	}

	r, ok := event.Resource.(types.AccessRequest)
	if !ok {
		p.log.Warn("Unknown event received, skipping.", "kind", event.Resource.GetKind(), "type", fmt.Sprintf("%T", event.Resource))
		return nil
	}

	switch r.GetState() {
	case types.RequestState_PENDING:
		p.log.Info("Received a new access request", "access_request_name", r.GetName())
	case types.RequestState_APPROVED:
		return p.reviewHandler.HandleReview(ctx, r)
	case types.RequestState_DENIED:
		return p.reviewHandler.HandleReview(ctx, r)
	default:
		p.log.Warn("Unknown access request state, skipping", "access_request_name", r.GetName(), "state", r.GetState())
	}

	return nil
}
