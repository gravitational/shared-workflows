package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/go-github/v71/github"
)

// Handler is an implementation of [http.Handler] that handles GitHub webhook events.
type Handler struct {
	eventHandler              EventHandler
	payloadValidationDisabled bool
	secretToken               []byte
	log                       *slog.Logger
}

var _ http.Handler = &Handler{}

// EventHandler is an interface that handles GitHub webhook events.
type EventHandler interface {
	// HandleEvent handles a GitHub webhook event.
	// The event is passed as an interface{} and should be type asserted to the appropriate type.
	HandleEvent(ctx context.Context, event any) error
}

// EventHandlerFunc is a function that handles a webhook event.
// The function should return an error if the event could not be handled.
// If the error is not nil, the webhook will respond with a 500 Internal Server Error.
//
// It is important that this is non-blocking and does not perform any long-running operations.
// The context passed to this function is tied to the request and will be cancelled when the request is done.
// GitHub will close the connection if the webhook does not respond within 10 seconds.
// When that connection is closed, the context will be cancelled.
//
// Example usage:
//
//	func(ctx context.Context, event any) error {
//		switch e := event.(type) {
//		case *github.CommitCommentEvent:
//			go processCommitCommentEvent(e)
//		case *github.CreateEvent:
//			go processCreateEvent(e)
//		default:
//			return fmt.Errorf("unsupported event type: %T", event)
//		}
//		return nil
//	}
type EventHandlerFunc func(ctx context.Context, event any) error

func (e EventHandlerFunc) HandleEvent(ctx context.Context, event any) error {
	return e(ctx, event)
}

type Opt func(*Handler) error

// WithSecretToken sets the secret token for the webhook.
// The secret token is used to create a hash of the request body, which is sent in the X-Hub-Signature header.
// If not set, the webhook will not verify the signature of the request.
//
// For more information, see: https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
func WithSecretToken(secretToken string) Opt {
	return func(p *Handler) error {
		p.secretToken = []byte(secretToken)
		return nil
	}
}

// WithoutPayloadValidation disables the signature validation for the webhook.
// By default, signature validation is enabled and a secret token is required.
//
// This should not be used in production.
// The main use case for this is to allow testing the webhook without a secret token.
func WithoutPayloadValidation() Opt {
	return func(p *Handler) error {
		p.payloadValidationDisabled = true
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

// NewHandler creates a new webhook handler that implements [http.Handler].
// The handler will call the eventHandler function when a webhook event is received.
// Example usage:
//
//	mux := http.NewServeMux()
//	mux.Handle("/webhook", webhook.NewHandler(
//		eventHandler,
//		webhook.WithLogger(logger),
//		... // Other options
//	))
func NewHandler(eventHandler EventHandler, opts ...Opt) (*Handler, error) {
	h := Handler{
		log:          slog.Default(),
		eventHandler: eventHandler,
	}
	for _, opt := range opts {
		opt(&h)
	}

	if !h.payloadValidationDisabled && len(h.secretToken) == 0 {
		return nil, fmt.Errorf("secret token is required")
	}

	// If the secret token is set, payload validation will always be enabled.
	if len(h.secretToken) > 0 {
		h.payloadValidationDisabled = false
	}

	if h.payloadValidationDisabled {
		h.log.Warn("payload validation is disabled, webhook will not verify signatures")
	}

	return &h, nil
}

// Headers is a list of special headers that are sent with a webhook request.
// For more information, see: https://docs.github.com/en/webhooks/webhook-events-and-payloads#delivery-headers
type Headers struct {
	// GitHubHookID is the unique identifier of the webhook.
	GitHubHookID string
	// GitHubEvent is the type of event that triggered the delivery.
	GitHubEvent string
	// GitHubDelivery is a globally unique identifier (GUID) to identify the event
	GitHubDelivery string
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
	head := Headers{
		GitHubHookID:                     r.Header.Get("X-GitHub-Hook-ID"),
		GitHubEvent:                      r.Header.Get("X-GitHub-Event"),
		GitHubDelivery:                   r.Header.Get("X-GitHub-Delivery"),
		GitHubHookInstallationTargetType: r.Header.Get("X-GitHub-Hook-Installation-Target-Type"),
		GitHubHookInstallationTargetID:   r.Header.Get("X-GitHub-Hook-Installation-Target-ID"),
		HubSignature256:                  r.Header.Get("X-Hub-Signature-256"),
	}

	// Signature is present but no secret token is set.
	// This indicates an issues with the webhook configuration.
	if len(h.secretToken) == 0 && head.HubSignature256 != "" {
		h.log.Error("received signature but no secret token is set", "github_headers", head)
		http.Error(w, "invalid request", http.StatusInternalServerError)
		return
	}

	payload, err := github.ValidatePayload(r, h.secretToken) // If secretToken is empty, the signature will not be verified.
	if err != nil {
		h.log.Warn("webhook validation failed", "github_headers", head, "error", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(head.GitHubEvent, payload)
	if err != nil {
		h.log.Error("failed to parse webhook event", "error", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.eventHandler.HandleEvent(r.Context(), event); err != nil {
		h.log.Error("failed to handle webhook event", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Respond to the request.
	w.WriteHeader(http.StatusOK)
}

// LogValue satisfies the [slog.LogValuer] interface.
// It presents a structured view of the headers for logging.
func (h *Headers) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("github_hook_id", h.GitHubHookID),
		slog.String("github_event", h.GitHubEvent),
		slog.String("github_delivery", h.GitHubDelivery),
		slog.String("github_hook_installation_target_type", h.GitHubHookInstallationTargetType),
		slog.String("github_hook_installation_target_id", h.GitHubHookInstallationTargetID),
		slog.String("hub_signature_256", h.HubSignature256),
	)
}
