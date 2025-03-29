package githubevents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/gravitational/shared-workflows/libs/github/webhook"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
)

// Source is a webhook that listens for GitHub events and processes them.
type Source struct {
	processor   DeploymentReviewEventProcessor
	addr        string
	secretToken string

	deployReviewChan chan *github.DeploymentReviewEvent
	listener         net.Listener
	srv              *http.Server

	validation map[string]struct{}

	log *slog.Logger
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

// Opt is a functional option for the GitHub event source.
type SourceOpt func(*Source)

// WithLogger sets the logger for the GitHub event source.
func WithLogger(logger *slog.Logger) SourceOpt {
	return func(s *Source) {
		s.log = logger
	}
}

var defaultOpts = []SourceOpt{
	WithLogger(slog.Default()),
}

func NewSource(cfg config.GitHubEvents, processor DeploymentReviewEventProcessor, opt ...SourceOpt) *Source {
	ghes := &Source{
		processor:   processor,
		addr:        cfg.Address,
		secretToken: cfg.Secret,
		validation:  map[string]struct{}{},
	}

	for _, o := range append(defaultOpts, opt...) {
		o(ghes)
	}

	for _, v := range cfg.Validation {
		for _, env := range v.Environments {
			ghes.validation[envEntry(v.Org, v.Repo, env)] = struct{}{}
		}
	}

	return ghes
}

// Setup GH client, webhook secret, etc.
// https://github.com/go-playground/webhooks may help here
func (ghes *Source) Setup(ctx context.Context) error {
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

	handler, err := webhook.NewHandler(eventProcessor, webhook.WithSecretToken(ghes.secretToken), webhook.WithLogger(ghes.log))
	if err != nil {
		return fmt.Errorf("error creating webhook handler: %w", err)
	}

	mux.Handle("/webhook", handler)

	listener, err := net.Listen("tcp", ghes.addr)
	if err != nil {
		return fmt.Errorf("error listening on address %q: %w", ghes.addr, err)
	}
	ghes.listener = listener
	ghes.srv = &http.Server{
		Addr:    listener.Addr().String(),
		Handler: mux,
	}

	return nil
}

// Take incoming events and respond to them
func (ghes *Source) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errC := make(chan error)

	// Start the HTTP server
	go func() {
		ghes.log.Info("Listening for GitHub Webhooks", "address", ghes.srv.Addr)
		errC <- ghes.srv.Serve(ghes.listener)
		close(errC)
	}()

	// Process incoming events
	go func() {
		ghes.log.Info("Starting GitHub event processor")
		for deployReview := range ghes.deployReviewChan {
			go ghes.processDeploymentReviewEvent(deployReview)
		}
	}()

	// Handle shutdown from context cancellation
	shutdownErrC := make(chan error)
	go func() {
		<-ctx.Done()
		close(ghes.deployReviewChan)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErrC <- ghes.srv.Shutdown(ctx)
		close(shutdownErrC)
	}()

	// This will block until an error occurs or the context is done
	select {
	case err := <-errC:
		cancel()
		return errors.Join(err, <-shutdownErrC)
	case <-ctx.Done():
		if err := <-shutdownErrC; err != nil {
			return fmt.Errorf("error shutting down server: %w", err)
		}
		return ctx.Err()
	}
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
func (ghes *Source) performAutomatedChecks(payload *github.DeploymentReviewEvent) (deny bool, err error) {
	org := payload.GetOrganization().GetLogin()
	repo := payload.GetRepo().GetName()
	env := payload.GetEnvironment()

	if _, ok := ghes.validation[envEntry(org, repo, env)]; !ok {
		return true, nil
	}

	// TODO: check user is part of one of valid orgs

	return false, nil
}

func (ghes *Source) getAddr() string {
	return ghes.listener.Addr().String()
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

func envEntry(org, repo, env string) string {
	return fmt.Sprintf("%s/%s/%s", org, repo, env)
}
