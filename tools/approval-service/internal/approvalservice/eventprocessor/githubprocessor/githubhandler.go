package githubprocessor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"text/template"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// Processor manages and responds to changes in state for Access Requests and how they relate to GitHub deployments.
// It is also responsible for updating the state of GitHub deployments based on the state of Access Requests.
type Processor struct {
	client     ghClient
	org        string
	repo       string
	envs       []string
	teleClient teleClient

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

var _ accessrequest.ReviewHandler = &Processor{}

// Opt is a function that modifies the GitHubHandler.
type Opt func(r *Processor) error

// WithLogger sets the logger for the GitHubHandler.
func WithLogger(log *slog.Logger) Opt {
	return func(r *Processor) error {
		r.log = log
		return nil
	}
}

var defaultOpts = []Opt{
	WithLogger(slog.Default()),
}

// New creates a new Processor.
func New(ctx context.Context, cfg config.GitHubSource, tele teleClient, opts ...Opt) (*Processor, error) {
	f, err := os.Open(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("opening private key: %w", err)
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()
	pKey, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	client, err := github.NewForApp(cfg.AppID, cfg.InstallationID, pKey)
	if err != nil {
		return nil, fmt.Errorf("initializing GitHub client: %w", err)
	}
	return newFromClient(client, cfg, tele, opts...)
}

func newFromClient(client ghClient, cfg config.GitHubSource, tele teleClient, opts ...Opt) (*Processor, error) {
	var p Processor

	for _, o := range append(defaultOpts, opts...) {
		if err := o(&p); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}
	p.client = client
	p.teleClient = tele
	p.org = cfg.Org
	p.repo = cfg.Repo
	p.envs = cfg.Environments

	return &p, nil
}

// FindExistingAccessRequest checks if an Access Request already exists for the given GitHub deployment review event.
func (p *Processor) FindExistingAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent) (types.AccessRequest, error) {
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

// CreateNewAccessRequest creates a new Access Request for the given GitHub deployment review event.
func (p *Processor) CreateNewAccessRequest(ctx context.Context, e githubevents.DeploymentReviewEvent, req types.AccessRequest) (types.AccessRequest, error) {
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
	err = p.storeData(req, &githubData{
		id:            req.GetName(),
		org:           e.Organization,
		repo:          e.Repository,
		env:           e.Environment,
		workflowRunID: e.WorkflowID,
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
func (p *Processor) automaticallyDenied(e githubevents.DeploymentReviewEvent) (deny bool, err error) {
	if e.Organization != p.org {
		return true, fmt.Errorf("organization %q does not match expected organization %q", e.Organization, p.org)
	}

	if e.Repository != p.repo {
		return true, fmt.Errorf("repository %q does not match expected repository %q", e.Repository, p.repo)
	}

	if !slices.Contains(p.envs, e.Environment) {
		return true, fmt.Errorf("environment %q does not match expected environments %q", e.Environment, p.envs)
	}

	// TODO: check user is part of one of valid orgs

	return false, nil
}

// HandleReview handles updates to the state of a Teleport Access Request and updates the state of the GitHub deployment accordingly.
// This implements the [accessrequest.ReviewHandler] interface.
func (p *Processor) HandleReview(ctx context.Context, req types.AccessRequest) error {
	p.log.Info("Handling review", "access_request_name", req.GetName())

	data, err := p.getData(req)
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

	p.log.Info("Updating deployment", "access_request_name", req.GetName(), "environment", data.env, "state", state)
	if err := p.client.ReviewDeploymentProtectionRule(ctx, data.org, data.repo, data.workflowRunID, state, data.env,
		fmt.Sprintf("Access by pipeline approval service. Access Request ID: %s", req.GetName())); err != nil {
		return fmt.Errorf("reviewing deployment protection rule: %w", err)
	}

	return nil
}

// githubData is a struct that holds the GitHub data for the access request.
// This state is expected to be stored externally (e.g. in a database) and not in memory.
// It is used to track the state of the access request and its associated GitHub data.
type githubData struct {
	id            string
	org           string
	repo          string
	env           string
	workflowRunID int64
}

func (p *Processor) storeData(req types.AccessRequest, data *githubData) error {
	labels := req.GetStaticLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels["workflow_run_id"] = strconv.Itoa(int(data.workflowRunID))
	labels["organization"] = data.org
	labels["repository"] = data.repo
	labels["environment"] = data.env
	req.SetStaticLabels(labels)

	return nil
}

func (p *Processor) getData(req types.AccessRequest) (*githubData, error) {
	runIDLabel := req.GetStaticLabels()["workflow_run_id"]
	if runIDLabel == "" {
		return nil, fmt.Errorf("missing workflow run ID in access request %s", req.GetName())
	}

	orgLabel := req.GetStaticLabels()["organization"]
	if orgLabel == "" {
		return nil, fmt.Errorf("missing organization in access request %s", req.GetName())
	}

	repoLabel := req.GetStaticLabels()["repository"]
	if repoLabel == "" {
		return nil, fmt.Errorf("missing repository in access request %s", req.GetName())
	}

	env := req.GetStaticLabels()["environment"]
	if env == "" {
		return nil, fmt.Errorf("missing environment in access request %s", req.GetName())
	}

	runID, err := strconv.Atoi(runIDLabel)
	if err != nil {
		return nil, fmt.Errorf("parsing workflow run ID: %w", err)
	}

	return &githubData{
		id:            req.GetName(),
		org:           orgLabel,
		repo:          repoLabel,
		env:           env,
		workflowRunID: int64(runID),
	}, nil
}

var reasonTmpl = template.Must(template.New("reason").Parse(`GitHub Deployment Review for:

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
