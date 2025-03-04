package github

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/gravitational/shared-workflows/libs/github/webhook"
)

var logger = slog.Default()

// GithubEventSource is a webhook that listens for GitHub events and processes them.
type GitHubEventSource struct {
	processor ApprovalProcessor

	deployReviewChan  chan *github.DeploymentReviewEvent
	addr              string
	srv               *http.Server
	validEnvironments []string
	validRepos        []string
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
		Addr:    ":8080", // TODO: get from config
		Handler: mux,
	}

	// TODO: get from config
	ghes.validEnvironments = []string{
		"prod/build",
		"prod/publish",
		"stage/build",
		"stage/publish",
	}

	// TODO: get from config
	ghes.validRepos = []string{
		"teleport.e",
	}

	return nil
}

// Take incoming events and respond to them
func (ghes *GitHubEventSource) Run(ctx context.Context) error {
	errc := make(chan error)

	// Start the HTTP server
	go func() {
		logger.Info("Listening for GitHub Webhooks", "address", ghes.srv.Addr)
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
		return fmt.Errorf("error encountered while running GitHub event source: %w", err)
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

	if !slices.Contains(ghes.validEnvironments, *payload.Environment) {
		return true, nil
	}

	if !slices.Contains(ghes.validRepos, *payload.Repo.Name) {
		return true, nil
	}

	return false, nil
}

func (ghes *GitHubEventSource) respondToDeployRequest(approved bool, payload interface{}) error {
	// TODO call GH API

	return nil
}
