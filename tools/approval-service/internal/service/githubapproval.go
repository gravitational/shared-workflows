/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/teleport/api/types"
)

// gitHubWorkflowApprover handles approvals/rejections for deployment protection reviews on workflows
// based on the reviewed Access Requests. This is per-repo, and contains the logic to handle the
// decision-making process for deployment protection rules.
type gitHubWorkflowApprover struct {
	ghClient ghClient
	org      string
	repo     string

	// envToRole maps GitHub environment names to Teleport roles.
	// This is used to determine which Teleport role to request when an Access Request is created.
	//
	// For example, we can have an environment "build-staging" that maps to the Teleport role "gha-env-build-staging".
	// When a workflow run requests "build-staging" environment, we will create an Access Request for the "gha-env-build-staging" role.
	envToRole map[string]string
	log       *slog.Logger
}

// ghClient is an interface that abstracts the GitHub client to allow for testing.
// This allows us to have a stub implementation without having to make real API calls.
type ghClient interface {
	GetWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (github.WorkflowRunInfo, error)
	ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, info github.ReviewDeploymentProtectionRuleInfo) error
}

// newGitHubWorkflowApprover creates a new GitHub deployment approval handler for deployment protection rules
// in a given GitHub organization and repository.
func newGitHubWorkflowApprover(ctx context.Context, cfg config.GitHubSource, client ghClient, log *slog.Logger) (*gitHubWorkflowApprover, error) {
	h := &gitHubWorkflowApprover{
		log:       log,
		org:       cfg.Org,
		repo:      cfg.Repo,
		envToRole: make(map[string]string),
		ghClient:  client,
	}

	for _, env := range cfg.Environments {
		h.envToRole[env.Name] = env.TeleportRole
	}

	return h, nil
}

// teleportRoleForEnvironment returns the Teleport role for a given environment.
func (h *gitHubWorkflowApprover) teleportRoleForEnvironment(env string) (string, error) {
	role, ok := h.envToRole[env]
	if !ok {
		return "", fmt.Errorf("no Teleport role configured for environment %q", env)
	}
	return role, nil
}

// handleDecisionForAccessRequestReviewed processes the decision for an access request that has been reviewed.
// It will either approve or reject the deployment protection rule based on the state of the access request
func (h *gitHubWorkflowApprover) handleDecisionForAccessRequestReviewed(ctx context.Context, req types.AccessRequest, info githubWorkflowLabels) error {
	decision := github.PendingDeploymentApprovalStateRejected
	if req.GetState() == types.RequestState_APPROVED {
		decision = github.PendingDeploymentApprovalStateApproved
	}

	// build comment for the deployment protection rule review
	authors := []string{}
	for _, reviews := range req.GetReviews() {
		authors = append(authors, reviews.Author)
	}
	comment := fmt.Sprintf("Access Request %q was %s by %s. This decision was made by the approval service based on Access Request state.",
		req.GetName(), decision, strings.Join(authors, ", "))

	// review the deployment protection rule with the decision and comment
	err := h.ghClient.ReviewDeploymentProtectionRule(ctx, h.org, h.repo,
		github.ReviewDeploymentProtectionRuleInfo{
			RunID:           info.WorkflowRunID,
			State:           decision,
			EnvironmentName: info.Env,
			Comment:         comment,
		},
	)
	if err != nil {
		return fmt.Errorf("reviewing deployment protection rule: %w", err)
	}

	h.log.Info("Handled decision for access request reviewed", "org", h.org, "repo", h.repo, "env", info.Env, "workflow_run_id", info.WorkflowRunID, "decision", decision)
	return nil
}

// generateAccessRequestReason generates a reason for the access request based on the workflow run information.
// This reason will be used in the access request to provide context for the approval.
func (h *gitHubWorkflowApprover) generateAccessRequestReason(runID int64, env string) (string, error) {
	runInfo, err := h.ghClient.GetWorkflowRunInfo(context.Background(), h.org, h.repo, runID)
	if err != nil {
		return "", fmt.Errorf("getting workflow run info: %w", err)
	}

	templateData := &accessRequestReasonTemplateData{
		Organization: h.org,
		Repository:   h.repo,
		WorkflowName: runInfo.Name,
		URL:          runInfo.HTMLURL,
		Environment:  env,
		WorkflowID:   runID,
		Requester:    runInfo.Requester,
	}

	var buff bytes.Buffer
	if err := reasonTmpl.Execute(&buff, templateData); err != nil {
		return "", fmt.Errorf("executing reason template: %w", err)
	}
	return buff.String(), nil
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

type accessRequestReasonTemplateData struct {
	Organization string
	Repository   string
	WorkflowName string
	URL          string
	Environment  string
	WorkflowID   int64
	Requester    string
}
