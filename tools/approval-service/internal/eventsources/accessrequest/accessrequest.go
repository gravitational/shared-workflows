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

package accessrequest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	teleportclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// EventWatcher watches and responds to changes in state of Access Requests.
// This is an implementation of a Teleport Access Request plugin.
// The underlying watcher polls the Teleport API for changes to Access Requests and generates an event stream.
type EventWatcher struct {
	teleportClient               *teleportclient.Client
	AccessRequestReviewedHandler AccessRequestReviewedHandler
	watch                        types.Watcher

	requesterFilter string

	log *slog.Logger
}

// AccessRequestReviewedHandler is an interface for handling approval and rejection events.
// When an Access Request is approved or denied, the plugin will call the HandleAccessRequestReviewed method.
// The handler should implement the logic to process the review, such as sending notifications or updating records.
type AccessRequestReviewedHandler interface {
	HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error
}

// Opt is an option for the Access Request plugin.
type Opt func(*EventWatcher) error

// WithLogger sets the logger for the plugin.
func WithLogger(logger *slog.Logger) Opt {
	return func(w *EventWatcher) error {
		if logger == nil {
			return fmt.Errorf("logger cannot be nil")
		}
		w.log = logger
		return nil
	}
}

// WithRequesterFilter sets a filter for the requester of Access Requests.
// This allows the plugin to only process requests from a specific user.
func WithRequesterFilter(requester string) Opt {
	return func(w *EventWatcher) error {
		if requester == "" {
			return fmt.Errorf("requester filter cannot be empty")
		}
		w.requesterFilter = requester
		return nil
	}
}

// NewEventWatcher creates a new Access Request plugin.
func NewEventWatcher(client *teleportclient.Client, handler AccessRequestReviewedHandler, opts ...Opt) (*EventWatcher, error) {
	if client == nil {
		return nil, errors.New("teleport client cannot be nil")
	}
	if handler == nil {
		return nil, errors.New("review handler cannot be nil")
	}

	p := &EventWatcher{
		teleportClient:               client,
		AccessRequestReviewedHandler: handler,
		log:                          slog.Default(),
	}

	for _, o := range opts {
		if err := o(p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}
	return p, nil
}

// Setup initializes the plugin by creating a new watcher for Access Requests.
func (w *EventWatcher) Setup(ctx context.Context) error {
	watch, err := w.teleportClient.NewWatcher(ctx, types.Watch{
		Kinds: []types.WatchKind{
			// AccessRequest is the resource we are interested in.
			{
				Kind:   types.KindAccessRequest,
				Filter: w.buildAccessRequestFilter(),
			},
		},
	})

	if err != nil {
		return fmt.Errorf("creating watcher from teleport client: %w", err)
	}
	w.watch = watch
	return nil
}

// Run starts processing events from the Teleport watcher.
// It will block until the context is cancelled.
func (w *EventWatcher) Run(ctx context.Context) (err error) {
	defer func() {
		err = errors.Join(err, w.watch.Close())
	}()

	w.log.Info("Starting the watcher job")

	for {
		select {
		case e := <-w.watch.Events():
			if err := w.handleEvent(ctx, e); err != nil {
				w.log.Error("Error handling event", "error", err)
			}
		case <-w.watch.Done():
			if err := w.watch.Error(); err != nil {
				return fmt.Errorf("watcher error: %w", err)
			}
			w.log.Info("The watcher job is finished")
			return nil
		}
	}
}

// handleEvent processes a single event received from the Teleport watcher.
// It checks the type of the event and delegates it to the appropriate handler.
func (w *EventWatcher) handleEvent(ctx context.Context, event types.Event) error {
	if event.Resource == nil {
		return nil
	}

	if _, ok := event.Resource.(*types.WatchStatusV1); ok {
		w.log.Info("Successfully started listening for Access Requests...")
		return nil
	}

	req, ok := event.Resource.(types.AccessRequest)
	if !ok {
		w.log.Warn("Unknown event received, skipping.", "kind", event.Resource.GetKind(), "type", fmt.Sprintf("%T", event.Resource))
		return nil
	}

	switch req.GetState() {
	case types.RequestState_PENDING:
		w.log.Info("Received a new access request", "access_request_name", req.GetName())
	case types.RequestState_APPROVED:
		return w.AccessRequestReviewedHandler.HandleAccessRequestReviewed(ctx, req)
	case types.RequestState_DENIED:
		return w.AccessRequestReviewedHandler.HandleAccessRequestReviewed(ctx, req)
	default:
		w.log.Warn("Unknown access request state, skipping", "access_request_name", req.GetName(), "state", req.GetState())
	}

	return nil
}

// buildAccessRequestFilter builds a filter for Access Requests based on the requester.
func (w *EventWatcher) buildAccessRequestFilter() map[string]string {
	m := map[string]string{}
	if w.requesterFilter != "" {
		m["requester"] = w.requesterFilter
	}

	return m
}
