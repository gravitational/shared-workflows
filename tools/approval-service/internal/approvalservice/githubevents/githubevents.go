package githubevents

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

// Source is a webhook that listens for GitHub events and processes them.
type Source struct {
	processor         DeploymentReviewEventProcessor
	addr              string
	validEnvironments []string
	validRepos        []string
	validOrgs         []string
	secretToken       string

	deployReviewChan chan *github.DeploymentReviewEvent
	srv              *http.Server
}

// Config is the configuration for the GitHub event source.
type Config struct {
	// Address is the address to listen for GitHub webhooks.
	Address string `json:"address,omitempty"`
	// ValidRepos is a list of valid repositories.
	ValidRepos []string `json:"valid_repos,omitempty"`
	// ValidEnvironments is a list of valid environments.
	ValidEnvironments []string `json:"valid_environments,omitempty"`
	// ValidOrgs is a list of valid organizations.
	ValidOrgs []string `json:"valid_orgs,omitempty"`
	// Secret is the secret used to authenticate the webhook.
	Secret string `json:"secret,omitempty"`
}

// DeploymentReviewEventProcessor is an interface for processing deployment review events.
type DeploymentReviewEventProcessor interface {
	// ProcessDeploymentReviewEvent processes a deployment review event.
	// Automated checks are done before this is called.
	// If the automated checks fail, valid will be false.
	ProcessDeploymentReviewEvent(event DeploymentReviewEvent, valid bool) error
}

// DeploymentReviewEvent is an event that is sent when a deployment review is requested.
// This contains information needed to process a request
// If multiple underlying events/payloads/etc. roll under
// the same "root" event that approval is for, they should
// all set the same ID.
type DeploymentReviewEvent struct {
	Requester    string
	Environment  string
	Organization string
	Repository   string
	WorkflowID   int64
	// Other fields TODO. Potential fields:
	// * Source
	// * Commit/tag/source control identifier
	// * "Parameters" map. In the case of GHA, this would be
	//   any input provided to a workflow dispatch event
	// See RFD for more details
}

func NewSource(cfg Config, processor DeploymentReviewEventProcessor) *Source {
	return &Source{
		processor:         processor,
		addr:              cfg.Address,
		validRepos:        cfg.ValidRepos,
		validEnvironments: cfg.ValidEnvironments,
		secretToken:       cfg.Secret,
	}
}

// Setup GH client, webhook secret, etc.
// https://github.com/go-playground/webhooks may help here
func (ghes *Source) Setup() error {
	deployReviewChan := make(chan *github.DeploymentReviewEvent)
	ghes.deployReviewChan = deployReviewChan

	mux := http.NewServeMux()
	eventProcessor := webhook.EventHandlerFunc(func(event any) error {
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
		webhook.WithSecretToken(ghes.secretToken),
		webhook.WithLogger(logger),
	))

	ghes.srv = &http.Server{
		Addr:    ghes.addr,
		Handler: mux,
	}

	return nil
}

// Take incoming events and respond to them
func (ghes *Source) Run(ctx context.Context) error {
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

// Process a deployment review event.
// This is where most of the business logic will go.
func (ghes *Source) processDeploymentReviewEvent(payload *github.DeploymentReviewEvent) error {
	// Do GitHub-specific checks. Don't approve based off ot this - just deny
	// if one fails.
	automatedDenial, err := ghes.performAutomatedChecks(payload)
	if err != nil {
		return fmt.Errorf("error performing automated checks: %w", err)
	}

	event := DeploymentReviewEvent{
		Requester:    payload.GetRequester().GetLogin(),
		Environment:  payload.GetEnvironment(),
		Organization: payload.GetOrganization().GetLogin(),
		Repository:   payload.GetRepo().GetName(),
		WorkflowID:   payload.GetWorkflowRun().GetWorkflowID(),
	}
	slog.Default().Info("automated checks", "valid", !automatedDenial, "event", event)

	// Process the event
	return ghes.processor.ProcessDeploymentReviewEvent(event, !automatedDenial)
}

// Performs approval checks that are GH-specific. This should only be used to deny requests,
// never approve them.
func (ghes *Source) performAutomatedChecks(payload *github.DeploymentReviewEvent) (pass bool, err error) {
	if !slices.Contains(ghes.validOrgs, payload.GetOrganization().GetLogin()) {
		return true, nil
	}

	if !slices.Contains(ghes.validEnvironments, payload.GetEnvironment()) {
		return true, nil
	}

	if !slices.Contains(ghes.validRepos, payload.GetRepo().GetName()) {
		return true, nil
	}

	// TODO: check user is part of one of valid orgs

	return false, nil
}

func (e DeploymentReviewEvent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("requester", e.Requester),
		slog.String("environment", e.Environment),
		slog.String("organization", e.Organization),
		slog.String("repository", e.Repository),
		slog.Int64("workflow_id", e.WorkflowID),
	)
}
