package eventprocessor

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventprocessor/store"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	teleportclient "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/types"
)

// WorkflowEventsProcessor processes GitHub Events and Teleport Access Request state changes that are related to GitHub workflows.
// At a high level, it waits for a GitHub Workflow to request access to an environment (signaled by deployment_protection_rule),
// and then creates a Teleport Access Request for the workflow.
// It then waits for the Access Request to be reviewed, and based on the review, it approves or rejects the workflow in GitHub.
type WorkflowEventsProcessor struct {
	store store.GitHubStorer

	// Required for creating new Access Requests.
	teleportUser    string
	requestTTLHours time.Duration
	teleClient      *teleportclient.Client

	// githubWorkflowsDecisionHandlers will be used to handle decisions to approve or reject deployment protection rules.
	// This is a map of GitHub organization/repository names to their respective decision handlers.
	// This allows us to handle multiple repositories with authentication and environment-specific logic.
	//
	// not safe for concurrent read/write
	// this is written to during init and only read concurrently during operation.
	githubWorkflowsDecisionHandlers map[string]*githubWorkflowsDecisionHandler

	// Channels for asynchronous processing of events.
	deployReviewEventC   chan githubevents.DeploymentReviewEvent
	accessRequestReviewC chan types.AccessRequest

	// For deduplication logic to ensure that we do not process similar events concurrently.
	// For example, we can receive 10s-100s of deployment review events in a short period of time for the same workflow.
	mu                  sync.Mutex
	currentlyProcessing map[string]struct{} // tracks currently processing events by their IDs

	log *slog.Logger
}

// Handles approvals/rejections for deployment protection reviews on workflows based on the reviewed Access Requests.
// This is per-repo, and contains the logic to handle the decision-making process for deployment protection rules.
type githubWorkflowsDecisionHandler struct {
	ghClient *github.Client
	org      string
	repo     string

	envToRole map[string]string
	log       *slog.Logger
}

// Opt is a functional option for configuring the Dispatcher.
type Opt func(d *WorkflowEventsProcessor) error

// NewWorkflowEventsProcessor creates a new Dispatcher instance.
func NewWorkflowEventsProcessor(cfg config.Root, teleClient *teleportclient.Client, opts ...Opt) (*WorkflowEventsProcessor, error) {
	store, err := store.NewGitHubStore()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub store: %w", err)
	}

	d := &WorkflowEventsProcessor{
		store:                           store,
		teleClient:                      teleClient,
		requestTTLHours:                 cmp.Or(time.Duration(cfg.ApprovalService.Teleport.RequestTTLHours)*time.Hour, 7*24*time.Hour),
		teleportUser:                    cfg.ApprovalService.Teleport.User,
		githubWorkflowsDecisionHandlers: make(map[string]*githubWorkflowsDecisionHandler),
		currentlyProcessing:             make(map[string]struct{}),
		log:                             slog.Default(),
		// Channels for asynchronous processing of events.
		// Setting buffer size to 1 to allow for non-blocking sends but still allow for some backpressure.
		// An issue is that for large buffers, we end up with a lot of events queued in memory that will be lost if the service crashes.
		// Due to the asynchronous nature, once the event is received from the channel, it is considered "processed" and GitHub will show the event as successful.
		// Balancing throughput and backpressure is important to avoid overwhelming the system and to properly handle event processing.
		deployReviewEventC:   make(chan githubevents.DeploymentReviewEvent, 1),
		accessRequestReviewC: make(chan types.AccessRequest, 1),
	}

	for _, repoCfg := range cfg.EventSources.GitHub {
		// Create a new decision handler for each repository.
		handler, err := newDeployProtectionRuleDecisionHandler(context.Background(), repoCfg, d.log)
		if err != nil {
			return nil, fmt.Errorf("creating deploy protection rule decision handler for %s/%s: %w", repoCfg.Org, repoCfg.Repo, err)
		}
		d.githubWorkflowsDecisionHandlers[githubRepoKey(repoCfg.Org, repoCfg.Repo)] = handler
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	return d, nil
}

// Run starts the Dispatcher and starts receiving events from the event sources.
// This handles fanning-in of events and asynchronously fanning-out to the appropriate consumers.
// It will block until the context is cancelled.
func (w *WorkflowEventsProcessor) Run(ctx context.Context) error {
	for {
		select {
		case deployReviewEvent := <-w.deployReviewEventC:
			go w.onDeploymentReviewEventReceived(ctx, deployReviewEvent)
		case accessRequestReview := <-w.accessRequestReviewC:
			go w.onAccessRequestReviewed(ctx, accessRequestReview)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// HandleDeploymentReviewEventReceived multiplexes deployment review events and handles them asynchronously.
// This function will return a nil error since the processing is done asynchronously.
func (w *WorkflowEventsProcessor) HandleDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) error {
	w.deployReviewEventC <- e
	return nil
}

// HandleWorkflowDispatchEventReceived is a placeholder for processing workflow dispatch events.
func (w *WorkflowEventsProcessor) HandleWorkflowDispatchEventReceived(ctx context.Context, e githubevents.WorkflowDispatchEvent) error {
	// This is not implemented yet.
	return fmt.Errorf("workflow_dispatch event processing is not implemented")
}

// HandleAccessRequestReviewed will handle updates to the state of a Teleport Access Request.
func (w *WorkflowEventsProcessor) HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error {
	w.accessRequestReviewC <- req
	return nil
}

func (w *WorkflowEventsProcessor) onDeploymentReviewEventReceived(ctx context.Context, e githubevents.DeploymentReviewEvent) {
	// One workflow can spawn multiple deployment review events (e.g. multiple jobs starting at the same time).
	// We need to deduplicate these events to avoid creating multiple Access Requests for the same workflow.
	// We use the workflow ID and the organization/repository as the unique identifier for the event
	eventID := fmt.Sprintf("%s/%s/%d", e.Organization, e.Repository, e.WorkflowID)
	if !w.willProcessEvent(eventID) {
		// Already processing this event, skip it.
		w.log.Debug("Skipping already processed event", "event_id", eventID)
		return
	}
	defer w.markEventProcessed(eventID)

	w.log.Info("Processing deployment review event", "event", e)
	existingRequest, err := w.findExistingAccessRequest(ctx, e)
	if err != nil {
		w.log.Error("Attempting to find existing access request", "error", err)
		return
	}

	if existingRequest != nil {
		// An access request already exists for this workflow run.
		// Check its state to determine the next steps.

		if existingRequest.GetState() == types.RequestState_PENDING {
			// Existing request is pending, no further action needed.
			w.log.Info("Found existing pending access request", "access_request_name", existingRequest.GetName(), "event", e)
			return
		}

		// Existing request is already reviewed (approved/rejected).
		// This happens when a previous job in the workflow has already triggered the review process.
		_ = w.HandleAccessRequestReviewed(ctx, existingRequest) // Async handling does not return an error.
		return
	}

	req, err := w.createAccessRequest(ctx, e)
	if err != nil {
		w.log.Error("Error creating new access request", "event", e, "error", err)
		return
	}

	w.log.Info("Created new access request", "access_request_name", req.GetName(), "event", e)
}

// findExistingAccessRequest checks if an Access Request already exists for the given GitHub deployment review event.
// It returns the Access Request if it exists, or nil if it does not.
// An error indicates a problem with the Teleport API, not that an Access Request does not exist.
//
// Three main things can determined from this:
//  1. If no Access Request exists, we need to create one.
//  2. If an Access Request exists, and is pending, no further action is needed.
//  3. If an Access Request exists, and is not pending, we can update the state of the GitHub deployment accordingly.
func (w *WorkflowEventsProcessor) findExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
	list, err := w.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{})
	if err != nil {
		return nil, fmt.Errorf("getting access requests: %w", err)
	}

	for _, req := range list {
		info, err := w.store.GetWorkflowInfo(ctx, req)
		if err != nil {
			w.log.Debug("failed to get workflow info for access request", "access_request_name", req.GetName(), "error", err)
			// Not all Access Requests will have workflow info, so we can ignore this error.
			continue
		}

		// Check if the Access Request matches the GitHub deployment review event.
		if info.Org == e.Organization && info.Repo == e.Repository && info.Env == e.Environment && info.WorkflowRunID == e.WorkflowID {
			w.log.Info("Found existing access request for deployment review event", "access_request_name", req.GetName(), "event", e)
			return req, nil
		}
	}

	// No existing access request found.
	return nil, nil
}

// createAccessRequest creates a new Access Request for the given GitHub deployment review event.
func (w *WorkflowEventsProcessor) createAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
	ghWorkflowsDecisions, ok := w.githubWorkflowsDecisionHandlers[githubRepoKey(e.Organization, e.Repository)]
	if !ok {
		return nil, fmt.Errorf("no GitHub repository decision handler found for organization %q and repository %q", e.Organization, e.Repository)
	}

	role, err := ghWorkflowsDecisions.teleportRoleForEnvironment(e.Environment)
	if err != nil {
		return nil, fmt.Errorf("getting Teleport role for environment %q: %w", e.Environment, err)
	}
	newReq, err := types.NewAccessRequest(uuid.NewString(), w.teleportUser, role)
	if err != nil {
		return nil, fmt.Errorf("generating new access request: %w", err)
	}
	newReq.SetExpiry(time.Now().Add(w.requestTTLHours))
	err = w.store.StoreWorkflowInfo(ctx, newReq, store.GitHubWorkflowInfo{
		Org:           e.Organization,
		Repo:          e.Repository,
		Env:           e.Environment,
		WorkflowRunID: e.WorkflowID,
	})
	if err != nil {
		return nil, fmt.Errorf("storing workflow info: %w", err)
	}

	reason, err := ghWorkflowsDecisions.genAccessRequestReason(e.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("generating access request reason: %w", err)
	}
	newReq.SetRequestReason(reason)

	created, err := w.teleClient.CreateAccessRequestV2(ctx, newReq)
	if err != nil {
		return nil, fmt.Errorf("creating access request: %w", err)
	}

	return created, nil
}

// willProcessEvent checks if the event is already being processed.
func (w *WorkflowEventsProcessor) willProcessEvent(eventID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.currentlyProcessing[eventID]; exists {
		// Already processing this event, skip it.
		return false
	}

	// Mark this event as currently being processed.
	w.currentlyProcessing[eventID] = struct{}{}
	return true
}

// markEventProcessed marks the event as processed, allowing it to be processed again in the future.
func (w *WorkflowEventsProcessor) markEventProcessed(eventID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.currentlyProcessing, eventID)
}

// onAccessRequestReviewed processes the Access Request review event.
func (w *WorkflowEventsProcessor) onAccessRequestReviewed(ctx context.Context, req types.AccessRequest) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	info, err := w.store.GetWorkflowInfo(ctx, req)
	if err != nil {
		// If we cannot find the workflow info, we cannot process the request.
		// This is likely due to the access request not having the required labels.
		w.log.Debug("Getting workflow info for access request", "access_request_name", req.GetName(), "error", err)
		return
	}

	decisionHandler, ok := w.githubWorkflowsDecisionHandlers[githubRepoKey(info.Org, info.Repo)]
	if !ok {
		w.log.Error("Couldn't find a configured GitHub Repository to handle access request", "access_request_name", req.GetName(), "org", info.Org, "repo", info.Repo)
		return
	}

	if err := decisionHandler.handleDecisionForAccessRequestReviewed(ctx, req.GetState(), info.Env, info.WorkflowRunID); err != nil {
		w.log.Error("Error handling access request reviewed", "access_request_name", req.GetName(), "error", err)
		return
	}
	w.log.Info("Handled access request reviewed", "access_request_name", req.GetName(), "org", info.Org, "repo", info.Repo)
}

// newDeployProtectionRuleDecisionHandler creates a new deploy protection rule decision handler for a given GitHub organization and repository.
func newDeployProtectionRuleDecisionHandler(ctx context.Context, cfg config.GitHubSource, log *slog.Logger) (*githubWorkflowsDecisionHandler, error) {
	key, err := os.ReadFile(cfg.Authentication.App.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key file %q: %w", cfg.Authentication.App.PrivateKeyPath, err)
	}

	client, err := github.NewForApp(ctx, cfg.Authentication.App.AppID, cfg.Authentication.App.InstallationID, key)
	if err != nil {
		return nil, fmt.Errorf("creating GitHub client for app: %w", err)
	}

	p := &githubWorkflowsDecisionHandler{
		log:       log,
		org:       cfg.Org,
		repo:      cfg.Repo,
		envToRole: make(map[string]string),
		ghClient:  client,
	}

	for _, env := range cfg.Environments {
		p.envToRole[env.Name] = env.TeleportRole
	}

	return p, nil
}

// teleportRoleForEnvironment returns the Teleport role for a given environment.
func (d *githubWorkflowsDecisionHandler) teleportRoleForEnvironment(env string) (string, error) {
	role, ok := d.envToRole[env]
	if !ok {
		return "", fmt.Errorf("no Teleport role configured for environment %q", env)
	}
	return role, nil
}

// handleDecisionForAccessRequestReviewed processes the decision for an access request that has been reviewed.
// It will either approve or reject the deployment protection rule based on the state of the access request
func (d *githubWorkflowsDecisionHandler) handleDecisionForAccessRequestReviewed(ctx context.Context, status types.RequestState, env string, workflowID int64) error {
	var decision github.PendingDeploymentApprovalState
	switch status {
	case types.RequestState_APPROVED:
		decision = github.PendingDeploymentApprovalStateApproved
	default:
		decision = github.PendingDeploymentApprovalStateRejected
	}

	if err := d.ghClient.ReviewDeploymentProtectionRule(ctx, d.org, d.repo, workflowID, decision, env, ""); err != nil {
		return fmt.Errorf("reviewing deployment protection rule: %w", err)
	}

	d.log.Info("Handled decision for access request reviewed", "org", d.org, "repo", d.repo, "env", env, "workflow_run_id", workflowID, "decision", decision)
	return nil
}

var reasonTmpl = template.Must(template.New("").Parse(`GitHub Deployment Review for:

Repository: {{ .Organization}}/{{ .Repository }}
Workflow name: {{ .WorkflowName }}
URL: {{ .URL }}
Environment: {{ .Environment }}
Workflow run ID: {{ .WorkflowID }}
Requester: {{ .Requester }}

This request was generated by the pipeline approval service.
`))

type tmplInfo struct {
	Organization string
	Repository   string
	WorkflowName string
	URL          string
	Environment  string
	WorkflowID   int64
	Requester    string
}

func (d *githubWorkflowsDecisionHandler) genAccessRequestReason(runID int64) (string, error) {
	runInfo, err := d.ghClient.GetWorkflowRunInfo(context.Background(), d.org, d.repo, runID)
	if err != nil {
		return "", fmt.Errorf("getting workflow run info: %w", err)
	}

	tmplInfo := &tmplInfo{
		Organization: d.org,
		Repository:   d.repo,
		WorkflowName: runInfo.Name,
		URL:          runInfo.HTMLURL,
		Environment:  "",
		WorkflowID:   runID,
		Requester:    runInfo.Requester,
	}

	var buff bytes.Buffer
	if err := reasonTmpl.Execute(&buff, tmplInfo); err != nil {
		return "", fmt.Errorf("executing reason template: %w", err)
	}
	return buff.String(), nil
}

// WithLogger sets the logger for the Dispatcher.
func WithLogger(logger *slog.Logger) Opt {
	return func(d *WorkflowEventsProcessor) error {
		if logger == nil {
			return errors.New("logger cannot be nil")
		}
		d.log = logger
		return nil
	}
}

func githubRepoKey(org, repo string) string {
	return org + "/" + repo
}
