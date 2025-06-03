package githubevents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	"github.com/gravitational/shared-workflows/libs/github/webhook"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
)

// Source is a webhook that listens for GitHub events and processes them.
type Source struct {
	processor          DeploymentReviewEventProcessor
	addr               string
	secretToken        string
	disableSecretToken bool

	deployReviewChan chan *github.DeploymentProtectionRuleEvent
	listener         net.Listener
	srv              *http.Server

	// used for extra validation of event source
	org          string
	repo         string
	environments []string

	log *slog.Logger
}

// DeploymentReviewEventProcessor is an interface for processing deployment review events.
type DeploymentReviewEventProcessor interface {
	// ProcessDeploymentReviewEvent processes a deployment review event.
	// Automated checks are done before this is called.
	// If the automated checks fail, valid will be false.
	ProcessDeploymentReviewEvent(ctx context.Context, event DeploymentReviewEvent, valid bool) error
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
}

// Opt is a functional option for the GitHub event source.
type SourceOpt func(*Source)

// WithLogger sets the logger for the GitHub event source.
func WithLogger(logger *slog.Logger) SourceOpt {
	return func(s *Source) {
		s.log = logger
	}
}

// DisableSecretToken disables the secret token for the webhook.
// Mainly used for unit testing and local development.
func DisableSecretToken() SourceOpt {
	return func(s *Source) {
		s.disableSecretToken = true
	}
}

var defaultOpts = []SourceOpt{
	WithLogger(slog.Default()),
}

func NewSource(cfg config.GitHubSource, processor DeploymentReviewEventProcessor, opt ...SourceOpt) *Source {
	ghes := &Source{
		processor:    processor,
		addr:         cfg.WebhookAddr,
		secretToken:  cfg.Secret,
		org:          cfg.Org,
		repo:         cfg.Repo,
		environments: cfg.Environments,
	}

	for _, o := range append(defaultOpts, opt...) {
		o(ghes)
	}

	return ghes
}

// Setup GH client, webhook secret, etc.
// https://github.com/go-playground/webhooks may help here
func (ghes *Source) Setup(ctx context.Context) error {
	deployReviewChan := make(chan *github.DeploymentProtectionRuleEvent)
	ghes.deployReviewChan = deployReviewChan

	mux := http.NewServeMux()
	eventProcessor := webhook.EventHandlerFunc(func(event any) error {
		switch event := event.(type) {
		case *github.DeploymentProtectionRuleEvent:
			deployReviewChan <- event
		default:
			ghes.log.Debug("unknown event type", "type", fmt.Sprintf("%T", event))
		}
		return nil
	})

	opts := []webhook.Opt{
		webhook.WithSecretToken(ghes.secretToken),
		webhook.WithLogger(ghes.log),
	}
	if ghes.disableSecretToken {
		opts = append(opts, webhook.DisableSecretToken())
	}
	handler, err := webhook.NewHandler(eventProcessor, opts...)
	if err != nil {
		return fmt.Errorf("error creating webhook handler: %w", err)
	}

	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	mux.Handle("POST /", handler)

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
			go func() {
				if err := ghes.processDeploymentReviewEvent(ctx, deployReview); err != nil {
					ghes.log.Error("Error processing deployment review event", "error", err)
				}
			}()
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
func (ghes *Source) processDeploymentReviewEvent(ctx context.Context, payload *github.DeploymentProtectionRuleEvent) error {
	// Do GitHub-specific checks. Don't approve based off ot this - just deny
	// if one fails.
	automatedDenial, err := ghes.performAutomatedChecks(payload)
	if err != nil {
		return fmt.Errorf("error performing automated checks: %w", err)
	}

	workflowID, err := extractWorkflowIDFromURL(payload.GetDeploymentCallbackURL())
	if err != nil {
		return fmt.Errorf("error extracting workflow ID from callback URL: %w", err)
	}

	event := DeploymentReviewEvent{
		Requester:    payload.GetDeployment().GetCreator().GetLogin(),
		Environment:  payload.GetEnvironment(),
		Organization: payload.GetOrganization().GetLogin(),
		Repository:   payload.GetRepo().GetName(),
		WorkflowID:   workflowID,
	}
	slog.Default().Info("automated checks", "valid", !automatedDenial, "event", event)

	// Process the event
	return ghes.processor.ProcessDeploymentReviewEvent(ctx, event, !automatedDenial)
}

// Performs approval checks that are GH-specific. This should only be used to deny requests,
// never approve them.
func (ghes *Source) performAutomatedChecks(payload *github.DeploymentProtectionRuleEvent) (deny bool, err error) {
	org := payload.GetOrganization().GetLogin()
	repo := payload.GetRepo().GetName()
	env := payload.GetEnvironment()

	if org != ghes.org {
		return true, fmt.Errorf("organization %q does not match expected organization %q", org, ghes.org)
	}
	if repo != ghes.repo {
		return true, fmt.Errorf("repository %q does not match expected repository %q", repo, ghes.repo)
	}
	if !slices.Contains(ghes.environments, env) {
		return true, fmt.Errorf("environment %q does not match expected environments %q", env, ghes.environments)
	}

	// TODO: check user is part of one of valid orgs

	return false, nil
}

// extractWorkflowIDFromURL extracts the workflow ID from the callback URL.
// The URL is in the format:
// https://api.github.com/repos/<org>/<repo>/actions/runs/<workflow_id>/deployment_protection_rule
func extractWorkflowIDFromURL(urlToParse string) (int64, error) {
	callbackURL, err := url.Parse(urlToParse)
	if err != nil {
		return 0, fmt.Errorf("error parsing callback URL: %w", err)
	}
	p := path.Clean(callbackURL.Path)
	parts := strings.Split(p, "/")
	if len(parts) != 8 {
		return 0, fmt.Errorf("invalid callback URL: %q", urlToParse)
	}

	i, err := strconv.Atoi(parts[6])
	if err != nil {
		return 0, fmt.Errorf("error converting workflow ID to int: %w", err)
	}
	return int64(i), nil
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
