package accessrequest

import (
	"context"
	"fmt"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// Plugin is an Access Request plugin that listens for events from Teleport.
type Plugin struct {
	TeleportClient *client.Client
	EventHandler   interface {
		HandleEvent(ctx context.Context, event types.Event) error
	}
}

func NewPlugin(client *client.Client) *Plugin {
	return &Plugin{
		TeleportClient: client,
		EventHandler:   &ghaEnvHandler{},
	}
}

// Run starts the plugin and listens for events.
// It will block until the context is cancelled.
func (p *Plugin) Run(ctx context.Context) error {
	watch, err := p.TeleportClient.NewWatcher(ctx, types.Watch{
		Kinds: []types.WatchKind{
			types.WatchKind{Kind: types.KindAccessRequest},
		},
	})

	if err != nil {
		return err
	}
	defer watch.Close()

	fmt.Println("Starting the watcher job")

	for {
		select {
		case e := <-watch.Events():
			if err := p.EventHandler.HandleEvent(ctx, e); err != nil {
				return fmt.Errorf("handling event: %w", err)
			}
		case <-watch.Done():
			fmt.Println("The watcher job is finished")
			return nil
		}
	}
}

type ghaEnvHandler struct {
}

func (h *ghaEnvHandler) HandleEvent(ctx context.Context, event types.Event) error {

	if event.Resource == nil {
		return nil
	}

	if _, ok := event.Resource.(*types.WatchStatusV1); ok {
		fmt.Println("Successfully started listening for Access Requests...")
		return nil
	}

	r, ok := event.Resource.(types.AccessRequest)
	if !ok {
		fmt.Printf("Unknown (%T) event received, skipping.\n", event.Resource)
		return nil
	}

	if r.GetState() == types.RequestState_PENDING {
		fmt.Println("Successfully created a row")
		return nil
	}

	if err := g.updateSpreadsheet(r); err != nil {
		return err
	}
	fmt.Println("Successfully updated a spreadsheet row")
	return nil
}
