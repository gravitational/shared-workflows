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

// githubWorkflowLabels holds information about a GitHub workflow run that is waiting for approval.
type githubWorkflowLabels struct {
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

func setWorkflowLabels(req types.AccessRequest, info githubWorkflowLabels) error {
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

func validateInputString(s string, maxLength int) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("string %q is not valid UTF-8", s)
	}
	if utf8.RuneCountInString(s) > maxLength {
		return fmt.Errorf("string %q is too long, maximum length is %d", s, maxLength)
	}
	return nil
}

func getWorkflowLabels(req types.AccessRequest) (githubWorkflowLabels, error) {
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
		return githubWorkflowLabels{}, newMissingLabelError(req, missingLabels...)
	}

	runIDInt, err := strconv.Atoi(runID)
	if err != nil {
		return githubWorkflowLabels{}, fmt.Errorf("parsing workflow run ID: %w", err)
	}

	return githubWorkflowLabels{
		Org:           org,
		Repo:          repo,
		Env:           env,
		WorkflowRunID: int64(runIDInt),
	}, nil
}

func (l githubWorkflowLabels) matchesEvent(e githubevents.DeploymentReviewEvent) bool {
	return l.Org == e.Organization && l.Repo == e.Repository && l.Env == e.Environment && l.WorkflowRunID == e.WorkflowID
}

func newMissingLabelError(req types.AccessRequest, labelKeys ...string) *MissingLabelError {
	return &MissingLabelError{
		RequestID: req.GetName(),
		LabelKeys: labelKeys,
	}
}
