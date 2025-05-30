package accessrequest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	teleportclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// Plugin is an Access Request plugin that listens for events from Teleport.
type Plugin struct {
	teleportClient *teleportclient.Client
	reviewHandler  ReviewHandler

	requesterFilter string

	log *slog.Logger
}

// ReviewHandler is an interface for handling approval and rejection events.
type ReviewHandler interface {
	HandleReview(ctx context.Context, req types.AccessRequest) error
}

// Opt is an option for the Access Request plugin.
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

func WithRequesterFilter(requester string) Opt {
	return func(p *Plugin) error {
		if requester == "" {
			return fmt.Errorf("requester filter cannot be empty")
		}
		p.requesterFilter = requester
		return nil
	}
}

// NewPlugin creates a new Access Request plugin.
func NewPlugin(client *teleportclient.Client, handler ReviewHandler, opts ...Opt) (*Plugin, error) {
	if client == nil {
		return nil, fmt.Errorf("teleport client cannot be nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("review handler cannot be nil")
	}

	p := &Plugin{
		teleportClient: client,
		reviewHandler:  handler,
		log:            slog.Default(),
	}

	for _, o := range opts {
		if err := o(p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}
	return p, nil
}

func (p *Plugin) Setup(ctx context.Context) error {
	return nil
}

// Run starts the plugin and listens for events.
// It will block until the context is cancelled.
func (p *Plugin) Run(ctx context.Context) (err error) {
	watch, err := p.teleportClient.NewWatcher(ctx, types.Watch{
		Kinds: []types.WatchKind{
			// AccessRequest is the resource we are interested in.
			{
				Kind:   types.KindAccessRequest,
				Filter: p.buildAccessRequestFilter(),
			},
		},
	})

	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, watch.Close())
	}()

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

func (p *Plugin) buildAccessRequestFilter() map[string]string {
	m := map[string]string{}
	if p.requesterFilter != "" {
		m["requester"] = p.requesterFilter
	}

	return m
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
