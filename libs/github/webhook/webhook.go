package webhook

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/go-github/v63/github"
)

// EventHandlerFunc is a function that handles a webhook event.
// The function should return an error if the event could not be handled.
// If the error is not nil, the webhook will respond with a 500 Internal Server Error.
//
// It is important that this is non-blocking and does not perform any long-running operations.
// GitHub will close the connection if the webhook does not respond within 10 seconds.
//
// Example usage:
//
//	func(event interface{}) error {
//		switch event := event.(type) {
//			case *github.CommitCommentEvent:
//				processCommitCommentEvent(event)
//			case *github.CreateEvent:
//				processCreateEvent(event)
//				...
//		}
//		return nil
//	}
type EventHandlerFunc func(event interface{}) error

// Handler is an implementation of [http.Handler] that handles GitHub webhook events.
type Handler struct {
	eventHandler EventHandlerFunc
	secretToken  []byte
	log          *slog.Logger
}

var _ http.Handler = &Handler{}

type Opt func(*Handler) error

// WithSecretToken sets the secret token for the webhook.
// The secret token is used to create a hash of the request body, which is sent in the X-Hub-Signature header.
// If not set, the webhook will not verify the signature of the request.
//
// For more information, see: https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
func WithSecretToken(secretToken []byte) Opt {
	return func(p *Handler) error {
		p.secretToken = secretToken
		return nil
	}
}

// WithLogger sets the logger for the webhook.
func WithLogger(log *slog.Logger) Opt {
	return func(p *Handler) error {
		p.log = log
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
}

// NewHandler creates a new webhook handler.
func NewHandler(eventHandler EventHandlerFunc, opts ...Opt) *Handler {
	h := Handler{
		eventHandler: eventHandler,
	}
	for _, opt := range defaultOpts {
		opt(&h)
	}

	for _, opt := range opts {
		opt(&h)
	}
	return &h
}

// Headers is a list of special headers that are sent with a webhook request.
// For more information, see: https://docs.github.com/en/webhooks/webhook-events-and-payloads#delivery-headers
type Headers struct {
	// GithubHookID is the unique identifier of the webhook.
	GithubHookID string
	// GithubEvent is the type of event that triggered the delivery.
	GithubEvent string
	// GithubDelivery is a globally unique identifier (GUID) to identify the event
	GithubDelivery string
	// GitHubHookInstallationTargetType is the type of resource where the webhook was created.
	GitHubHookInstallationTargetType string
	// GitHubHookInstallationTargetID is the unique identifier of the resource where the webhook was created.
	GitHubHookInstallationTargetID string

	// HubSignature256 is the HMAC hex digest of the response body.
	// Is generated with the SHA-256 algorithm with a shared secret used as the HMAC key.
	// This header will be sent if the webhook is configured with a secret.
	HubSignature256 string
}

// ServeHTTP handles a webhook request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse headers for debugging and audit purposes.
	var head Headers
	head.GithubHookID = r.Header.Get("X-GitHub-Hook-ID")
	head.GithubEvent = r.Header.Get("X-GitHub-Event")
	head.GithubDelivery = r.Header.Get("X-GitHub-Delivery")
	head.GitHubHookInstallationTargetType = r.Header.Get("X-GitHub-Hook-Installation-Target-Type")
	head.GitHubHookInstallationTargetID = r.Header.Get("X-GitHub-Hook-Installation-Target-ID")
	head.HubSignature256 = r.Header.Get("X-Hub-Signature-256")

	payload, err := github.ValidatePayload(r, h.secretToken) // If secretToken is empty, the signature will not be verified.
	if err != nil {
		h.log.Warn("webhook validation failed", "headers", head)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(head.GithubEvent, payload)
	if err != nil {
		h.log.Error("failed to parse webhook event", "error", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.eventHandler(event); err != nil {
		h.log.Error("failed to handle webhook event", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Respond to the request.
	w.WriteHeader(http.StatusOK)
}

// String returns a string representation of the Headers.
func (h *Headers) String() string {
	return fmt.Sprintf("GithubHookID: %s\nGithubEvent: %s\nGithubDelivery: %s\nGitHubHookInstallationTargetType: %s\nGitHubHookInstallationTargetID: %s\nHubSignature256: %s\n",
		h.GithubHookID, h.GithubEvent, h.GithubDelivery, h.GitHubHookInstallationTargetType, h.GitHubHookInstallationTargetID, h.HubSignature256)
}
