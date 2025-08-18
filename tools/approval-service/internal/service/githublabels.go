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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// GithubWorkflowLabels holds information about a GitHub workflow run that is waiting for approval.
// These labels will be added as additional metadata to the Access Request created for the workflow run.
// This can be used to tie the Access Request to the specific workflow run and environment in GitHub.
type GithubWorkflowLabels struct {
	// Org is the GitHub organization name.
	Org string
	// Repo is the GitHub repository name.
	Repo string
	// Env is the environment name for the deployment.
	Env string
	// WorkflowRunID is the ID of the workflow run that is waiting for approval.
	WorkflowRunID int64
}

// MissingLabelError is returned when a required label is missing from the access request.
// This error is used to indicate that the access request does not contain the necessary labels to retrieve or store GitHub workflow information.
// In some cases, this is not an error, but rather a signal that the access request needs to be updated with the required labels.
type MissingLabelError struct {
	// RequestID is the ID of the access request that is missing the label.
	RequestID string
	// LabelsKeys is the list of label keys that are required but missing from the access request.
	LabelKeys []string
}

// Error implements the error interface for MissingLabelError.
func (m *MissingLabelError) Error() string {
	return fmt.Sprintf("access request %q is missing required labels: %s", m.RequestID, strings.Join(m.LabelKeys, ", "))
}

const (
	workflowRunLabel  = "workflow_run_id"
	organizationLabel = "organization"
	repositoryLabel   = "repository"
	environmentLabel  = "environment"
)

// setWorkflowLabels sets the GitHub workflow labels on the access request.
// This will serialize the GitHub workflow information into the Access Request labels as additional metadata.
// This is used to tie the access request to the specific workflow run and environment in GitHub.
//
// It also performs validation on the labels to ensure they are valid and within reasonable limits to
// prevent abuse of the Teleport API.
func setWorkflowLabels(req types.AccessRequest, info GithubWorkflowLabels) error {
	if info.Org == "" {
		return errors.New("GitHub organization cannot be empty")
	}
	if info.Repo == "" {
		return errors.New("GitHub repository cannot be empty")
	}
	if info.Env == "" {
		return errors.New("environment cannot be empty")
	}

	// Hard to check if WorkflowRunID is valid but at least check if it's a positive integer.
	if info.WorkflowRunID <= 0 {
		return fmt.Errorf("invalid workflow run ID: %d", info.WorkflowRunID)
	}

	// Validate the individual fields for length and UTF-8 validity.
	// This is a public API so we want to ensure that strings are valid UTF-8 and within reasonable length limits.
	if err := validateInputString(info.Org, 15); err != nil {
		return fmt.Errorf("invalid organization name: %w", err)
	}
	if err := validateInputString(info.Repo, 100); err != nil {
		return fmt.Errorf("invalid repository name: %w", err)
	}
	if err := validateInputString(info.Env, 30); err != nil {
		return fmt.Errorf("invalid environment name: %w", err)
	}

	labels := req.GetStaticLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[workflowRunLabel] = strconv.Itoa(int(info.WorkflowRunID))
	labels[organizationLabel] = info.Org
	labels[repositoryLabel] = info.Repo
	labels[environmentLabel] = info.Env
	req.SetStaticLabels(labels)

	return nil
}

// validateInputString checks if the input string is valid UTF-8 and does not exceed the maximum length.
// This is used to ensure that the labels we set on the Access Request are valid and do not cause issues with the Teleport API.
func validateInputString(s string, maxLength int) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("string %q is not valid UTF-8", s)
	}
	if utf8.RuneCountInString(s) > maxLength {
		return fmt.Errorf("string %q is too long, maximum length is %d", s, maxLength)
	}
	return nil
}

// getWorkflowLabels extracts the GitHub workflow labels from the access request.
// The labels are expected to be set by the `setWorkflowLabels` function and will contains information
// to tie to a specific GitHub workflow run and environment.
//
// When an Access Request is approved or denied, these labels will be used to determine the appropriate
// GitHub deployment protection rule to approve or reject.
func getWorkflowLabels(req types.AccessRequest) (GithubWorkflowLabels, error) {
	missingLabels := []string{}
	labels := req.GetStaticLabels()

	runID := labels[workflowRunLabel]
	if runID == "" {
		missingLabels = append(missingLabels, workflowRunLabel)
	}

	org := labels[organizationLabel]
	if org == "" {
		missingLabels = append(missingLabels, organizationLabel)
	}

	repo := labels[repositoryLabel]
	if repo == "" {
		missingLabels = append(missingLabels, repositoryLabel)
	}

	env := labels[environmentLabel]
	if env == "" {
		missingLabels = append(missingLabels, environmentLabel)
	}

	if len(missingLabels) > 0 {
		return GithubWorkflowLabels{}, newMissingLabelError(req, missingLabels...)
	}

	runIDInt, err := strconv.Atoi(runID)
	if err != nil {
		return GithubWorkflowLabels{}, fmt.Errorf("parsing workflow run ID: %w", err)
	}

	return GithubWorkflowLabels{
		Org:           org,
		Repo:          repo,
		Env:           env,
		WorkflowRunID: int64(runIDInt),
	}, nil
}

// matchesEvent is a helper function to check if the GitHub workflow labels match a given deployment review event.
// This can be used to determine is a received event already has a corresponding Access Request created for it.
func (l GithubWorkflowLabels) matchesEvent(e githubevents.DeploymentReviewEvent) bool {
	return l.Org == e.Organization && l.Repo == e.Repository && l.Env == e.Environment && l.WorkflowRunID == e.WorkflowID
}

func newMissingLabelError(req types.AccessRequest, labelKeys ...string) *MissingLabelError {
	return &MissingLabelError{
		RequestID: req.GetName(),
		LabelKeys: labelKeys,
	}
}
