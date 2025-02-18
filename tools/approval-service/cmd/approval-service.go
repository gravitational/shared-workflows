package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/gravitational/shared-workflows/libs/github/webhook"
	"golang.org/x/sync/errgroup"
)

var logger = slog.Default()

// Process:
// 1. Take in events from CI/CD systems
// 2. Extract common information
// 3. Process information according to business rules/logic
// 4. Callback to the event source, have it handle

// One of the design goals of this is to support multiple "sources" of deployment events,
// such as github or another CI/CD service.

// Skeleton TODO:
// * add ctx where needed
// * err handling
// * pass some form of "config" struct to setup funcs, which will be populated by CLI or config file
// * maybe add some "hook" for registering CLI options?
// * Move approval processor, event, and event source to different packages

func main() {
	// 0. Process CLI args, setup logger, etc.
	// TODO

	// 1. Setup approval processor
	var processor ApprovalProcessor = &TeleportApprovalProcessor{}
	_ = processor.Setup() // Error handling TODO

	// 2. Setup event sources
	eventSources := []EventSource{
		NewGitHubEventSource(processor),
	}
	for _, eventSource := range eventSources {
		_ = eventSource.Setup()
	}

	// 3. Start event sources
	eg, ctx := errgroup.WithContext(context.Background())
	for _, eventSource := range eventSources {
		eg.Go(func() error {
			return eventSource.Run(ctx)
		})
	}

	// Block until an event source has a fatal error
	if err := eg.Wait(); err != nil {
		log.Fatal(err)
	}
}

// This contains information needed to process a request
// If multiple underlying events/payloads/etc. roll under
// the same "root" event that approval is for, they should
// all set the same ID.
type Event struct {
	// Unique for an approval request, but may be common for
	// multiple underlying events/payloads/etc.
	ID        string
	Requester string
	// Other fields TODO. Potential fields:
	// * Source
	// * Commit/tag/source control identifier
	// * "Parameters" map. In the case of GHA, this would be
	//   any input provided to a workflow dispatch event
	// See RFD for more details
}

type ApprovalProcessor interface {
	// This should do things like setup API clients, as well as anything
	// needed to approve/deny events.
	Setup() error

	// This should be a blocking function that takes in an event, and
	// approves or denies it.
	ProcessRequest(*Event) (approved bool, err error)
}

type TeleportApprovalProcessor struct {
	// TODO
}

func (tap *TeleportApprovalProcessor) Setup() error {
	// Setup Teleport API client
	return nil
}

func (tap *TeleportApprovalProcessor) ProcessRequest(e *Event) (approved bool, err error) {
	// 1. Create a new role:
	// 	* Set TTL to value in RFD
	// 	* Encode event information in role for recordkeeping

	// 2. Request access to the role. Include the same info as the role,
	//    for reviewer visibility.

	// 3. Wait for the request to be approved or denied.
	// This may block for a long time (minutes, hours, days).
	// Timeout if it takes too long.

	return false, nil
}

type EventSource interface {
	// This should do thinks like setup API clients and webhooks.
	Setup() error

	// Handle actual requests. This should not block.
	Run(ctx context.Context) error
}

type GitHubEventSource struct {
	processor ApprovalProcessor

	deployReviewChan chan *github.DeploymentReviewEvent
	addr             string
	srv              *http.Server
}

func NewGitHubEventSource(processor ApprovalProcessor) *GitHubEventSource {
	return &GitHubEventSource{processor: processor}
}

// Setup GH client, webhook secret, etc.
// https://github.com/go-playground/webhooks may help here
func (ghes *GitHubEventSource) Setup() error {
	deployReviewChan := make(chan *github.DeploymentReviewEvent)
	ghes.deployReviewChan = deployReviewChan

	mux := http.NewServeMux()
	eventProcessor := webhook.EventHandlerFunc(func(event interface{}) error {
		switch event := event.(type) {
		case *github.DeploymentReviewEvent:
			deployReviewChan <- event
			return nil
		default:
			return fmt.Errorf("unknown event type: %T", event)
		}
	})
	mux.Handle("/webhook", webhook.NewHandler(
		eventProcessor,
		webhook.WithSecretToken("secret-token"), // TODO: get from config
		webhook.WithLogger(logger),
	))

	ghes.srv = &http.Server{
		Addr:    ghes.addr,
		Handler: mux,
	}

	return nil
}

// Take incoming events and respond to them
func (ghes *GitHubEventSource) Run(ctx context.Context) error {
	errc := make(chan error)

	// Start the HTTP server
	go func() {
		logger.Info("Listening for GitHub Webhooks", "address", ghes.addr)
		errc <- ghes.srv.ListenAndServe()
		close(errc)
	}()

	// Process incoming events
	go func() {
		defer close(ghes.deployReviewChan)
		logger.Info("Starting GitHub event processor")
		for {
			select {
			case <-ctx.Done():
				return
			case deployReview := <-ghes.deployReviewChan:
				// Process the event
				go ghes.processDeploymentReviewEvent(deployReview)
			}
		}
	}()

	var err error
	// This will block until an error occurs or the context is done
	select {
	case err = <-errc:
		ghes.srv.Shutdown(context.Background()) // Ignore error - we're already handling one
	case <-ctx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = ghes.srv.Shutdown(ctx)
		<-errc // flush the error channel to avoid a goroutine leak
	}

	if err != nil {
		return fmt.Errorf("error enconutered while running GitHub event source: %w", err)
	}
	return nil
}

func (ghes *GitHubEventSource) processDeploymentReviewEvent(payload *github.DeploymentReviewEvent) error {
	// Do GitHub-specific checks. Don't approve based off ot this - just deny
	// if one fails.
	automatedDenial, err := ghes.performAutomatedChecks(payload)
	if automatedDenial || err != nil {
		ghes.respondToDeployRequest(false, payload)
	}

	// Convert it to a generic event that is common to all sources
	event := ghes.convertWebhookPayloadToEvent(payload)

	// Process the event
	processorApproved, err := ghes.processor.ProcessRequest(event)

	// Respond to the event
	if !processorApproved || err != nil {
		ghes.respondToDeployRequest(false, payload)
	}

	_ = ghes.respondToDeployRequest(true, payload)
	return nil
}

// Given an event, approve or deny it. This is a long running, blocking function.
func (ghes *GitHubEventSource) processWebhookPayload(payload interface{}, done chan struct{}) {
}

// Turns GH-specific information into "common" information for the approver
func (ghes *GitHubEventSource) convertWebhookPayloadToEvent(payload interface{}) *Event {
	// This needs to perform logic to get the top-level workflow identifier. For example if
	// workflow job A calls workflow B, events for both A and B should use A's ID as the
	// event identifier

	return &Event{}
}

// Performs approval checks that are GH-specific. This should only be used to deny requests,
// never approve them.
func (ghes *GitHubEventSource) performAutomatedChecks(payload *github.DeploymentReviewEvent) (pass bool, err error) {
	// Verify request is from Gravitational org repo
	// Verify request is from Gravitational org member
	// See RFD for additional examples
	if *payload.Organization.Login != "gravitational" {
		return true, nil
	}

	return false, nil
}

func (ghes *GitHubEventSource) respondToDeployRequest(approved bool, payload interface{}) error {
	// TODO call GH API

	return nil
}
