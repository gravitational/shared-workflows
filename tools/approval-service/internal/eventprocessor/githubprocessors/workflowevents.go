package githubprocessors

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"text/template"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventprocessor/store"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// WorkflowEventsProcessor listens to events related to GitHub Workflows and manages the state changes of associated resources.
// Its main responsibility is to manage the interaction between GitHub Workflows and Teleport Access Requests.
// This entails orchestrating approvals/rejections state between GitHub Deployment Protection Rules and Teleport Access Requests.
//
// It doesn't only respond to events from GitHub, but also events from Teleport Access Requests that are related to GitHub deployments.
// For example, when a Teleport Access Request is created, this processor should receive future review events for that Access Request
// and update the state of the GitHub Workflow accordingly.
type WorkflowEventsProcessor interface {
	// FindExistingAccessRequest checks if an Access Request already exists for the given GitHub deployment review event.
	// It returns the Access Request if it exists, or nil if it does not.
	// An error indicates a problem with the Teleport API, not that an Access Request does not exist.
	//
	// Three main things can determined from this:
	// 	1. If no Access Request exists, we need to create one.
	//  2. If an Access Request exists, and is pending, no further action is needed.
	//  3. If an Access Request exists, and is not pending, we can update the state of the GitHub deployment accordingly.
	FindExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error)
	// CreateAccessRequest creates a new Access Request for the given GitHub deployment review event.
	// It returns the created Access Request or an error if the creation failed.
	CreateAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent, req types.AccessRequest) (types.AccessRequest, error)
	// TeleportRoleForEnvironment returns the Teleport role to request for the given environment.
	TeleportRoleForEnvironment(env string) (string, error)

	// Also implements the AccessRequestReviewedHandler interface to handle updates to the state of Teleport Access Requests.
	accessrequest.AccessRequestReviewedHandler
}

type workflowEventsProcessor struct {
	client     ghClient
	org        string
	repo       string
	envToRole  map[string]string
	teleClient teleClient
	store      store.GitHubService

	log *slog.Logger
}

// internal interface for the GitHub client to allow for easier tests.
// This is a subset of the github.Client interface that we need for our purposes.
// It is not intended to be a complete representation of the GitHub API.
type ghClient interface {
	ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, runID int64, state github.PendingDeploymentApprovalState, envName, comment string) error
	GeWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (github.WorkflowRunInfo, error)
}

// Small interface to allow for easier testing of the Teleport client.
// This is a subset of the teleport.Client interface that we need for our purposes.
// It is not intended to be a complete representation of the Teleport API or the teleport.Client implementation.
type teleClient interface {
	CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error)
	GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error)
}

var _ accessrequest.AccessRequestReviewedHandler = &workflowEventsProcessor{}

// Opt is a function that modifies the GitHubHandler.
type Opt func(r *workflowEventsProcessor) error

// WithLogger sets the logger for the GitHubHandler.
func WithLogger(log *slog.Logger) Opt {
	return func(r *workflowEventsProcessor) error {
		r.log = log
		return nil
	}
}

// NewWorkflowEventsProcessor creates a new WorkflowEventsProcessor for handling GitHub workflow events.
func NewWorkflowEventsProcessor(ctx context.Context, cfg config.GitHubSource, tele teleClient, store store.GitHubService, opts ...Opt) (WorkflowEventsProcessor, error) {
	key, err := os.ReadFile(cfg.Authentication.App.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key file %q: %w", cfg.Authentication.App.PrivateKeyPath, err)
	}

	client, err := github.NewForApp(ctx, cfg.Authentication.App.AppID, cfg.Authentication.App.InstallationID, key)
	if err != nil {
		return nil, fmt.Errorf("creating GitHub client for app: %w", err)
	}

	p := &workflowEventsProcessor{
		store:      store,
		teleClient: tele,
		log:        slog.Default(),
		org:        cfg.Org,
		repo:       cfg.Repo,
		envToRole:  make(map[string]string),
		client:     client,
	}

	for _, o := range opts {
		if err := o(p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	for _, env := range cfg.Environments {
		p.envToRole[env.Name] = env.TeleportRole
	}

	return p, nil
}

// FindExistingAccessRequest checks if an Access Request already exists for the given GitHub deployment review event.
func (p *workflowEventsProcessor) FindExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
	list, err := p.teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{})
	if err != nil {
		return nil, fmt.Errorf("getting access requests: %w", err)
	}
	for _, req := range list {
		labels := req.GetStaticLabels()
		if labels == nil {
			continue
		}
		if labels["organization"] == e.Organization && labels["repository"] == e.Repository && labels["environment"] == e.Environment && labels["workflow_run_id"] == strconv.Itoa(int(e.WorkflowID)) {
			return req, nil
		}
	}

	// No existing access request found.
	return nil, nil
}

// CreateAccessRequest creates a new Access Request for the given GitHub deployment review event.
func (p *workflowEventsProcessor) CreateAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent, req types.AccessRequest) (types.AccessRequest, error) {
	// Get the workflow run info from GitHub.
	runInfo, err := p.client.GeWorkflowRunInfo(ctx, e.Organization, e.Repository, e.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("getting workflow run info: %w", err)
	}

	reason, err := genReason(e, runInfo)
	if err != nil {
		return nil, fmt.Errorf("generating reason template: %w", err)
	}
	req.SetRequestReason(reason)
	err = p.store.StoreWorkflowInfo(ctx, req, store.GitHubWorkflowInfo{
		Org:           e.Organization,
		Repo:          e.Repository,
		Env:           e.Environment,
		WorkflowRunID: e.WorkflowID,
	})
	if err != nil {
		return nil, fmt.Errorf("storing data: %w", err)
	}

	newReq, err := p.teleClient.CreateAccessRequestV2(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating access request: %w", err)
	}

	_, denyErr := p.automaticallyDenied(e)
	if denyErr != nil {
		// TODO: Consider auto denying the request here.
		return nil, fmt.Errorf("automatically denied: %w", denyErr)
	}

	return newReq, nil
}

// Performs approval checks that are GH-specific. This should only be used to deny requests,
// never approve them.
func (p *workflowEventsProcessor) automaticallyDenied(e githubevents.DeploymentReviewEvent) (deny bool, err error) {
	if e.Organization != p.org {
		return true, fmt.Errorf("organization %q does not match expected organization %q", e.Organization, p.org)
	}

	if e.Repository != p.repo {
		return true, fmt.Errorf("repository %q does not match expected repository %q", e.Repository, p.repo)
	}

	if _, ok := p.envToRole[e.Environment]; !ok {
		return true, fmt.Errorf("environment %q does not match configured environments", e.Environment)
	}

	// TODO: check user is part of one of valid orgs

	return false, nil
}

func (p *workflowEventsProcessor) TeleportRoleForEnvironment(env string) (string, error) {
	if role, ok := p.envToRole[env]; ok {
		return role, nil
	}
	return "", fmt.Errorf("no teleport role found for environment %q", env)
}

// HandleAccessRequestReviewed handles updates to the state of a Teleport Access Request and updates the state of the GitHub deployment accordingly.
// This implements the [accessrequest.AccessRequestReviewedHandler] interface.
func (p *workflowEventsProcessor) HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error {
	p.log.Info("Handling review", "access_request_name", req.GetName())

	data, err := p.store.GetWorkflowInfo(ctx, req)
	if err != nil {
		return fmt.Errorf("getting data: %w", err)
	}

	var state github.PendingDeploymentApprovalState
	switch req.GetState() {
	case types.RequestState_APPROVED:
		state = github.PendingDeploymentApprovalStateApproved
	case types.RequestState_DENIED:
		state = github.PendingDeploymentApprovalStateRejected
	default:
		return fmt.Errorf("invalid state %q for access request %q", req.GetState(), req.GetName())
	}

	p.log.Info("Updating deployment", "access_request_name", req.GetName(), "environment", data.Env, "state", state)
	if err := p.client.ReviewDeploymentProtectionRule(ctx, data.Org, data.Repo, data.WorkflowRunID, state, data.Env,
		fmt.Sprintf("Access by pipeline approval service. Access Request ID: %s", req.GetName())); err != nil {
		return fmt.Errorf("reviewing deployment protection rule: %w", err)
	}

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

func genReason(e githubevents.DeploymentReviewEvent, runInfo github.WorkflowRunInfo) (string, error) {
	// No need to validate, the template will fail with an appropriate error if the fields are not set.
	tmplInfo := &tmplInfo{
		Organization: e.Organization,
		Repository:   e.Repository,
		Environment:  e.Environment,
		WorkflowID:   e.WorkflowID,
		Requester:    e.Requester,
		WorkflowName: runInfo.Name,
		URL:          runInfo.HTMLURL,
	}

	var buff bytes.Buffer
	if err := reasonTmpl.Execute(&buff, tmplInfo); err != nil {
		return "", err
	}
	return buff.String(), nil
}
