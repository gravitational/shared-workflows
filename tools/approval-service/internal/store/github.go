package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gravitational/teleport/api/types"
)

// GitHubWorkflowInfo holds information about a GitHub workflow run that is waiting for approval.
type GitHubWorkflowInfo struct {
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

func SetWorkflowInfoLabels(ctx context.Context, req types.AccessRequest, info GitHubWorkflowInfo) error {
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

	if err := validateUserInput(info); err != nil {
		return errors.New("invalid GitHub workflow info")
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

// validateUserInput checks each field for validity
// The info we gather is user input and part of a public API so we need to sanitize the data.
// A malicious user could launch a denial of service attack by sending very long strings or invalid UTF-8 sequences.
// We still allow some flexibility in the length of the strings, but we enforce a maximum length to prevent abuse.
// This does NOT mean that we are validating the content of the strings, only that they are valid UTF-8 and within reasonable length limits.
func validateUserInput(info GitHubWorkflowInfo) error {
	// GitHub webhook events are UTF-8 encoded.
	if !utf8.ValidString(info.Org) || !utf8.ValidString(info.Repo) || !utf8.ValidString(info.Env) {
		return errors.New("GitHub organization, repository, and environment must be valid UTF-8 strings")
	}

	// Low limit but we only use for gravitational so we can assume that the org name is not too long.
	if utf8.RuneCountInString(info.Org) > 15 {
		return fmt.Errorf("GitHub organization name is too long: %s", info.Org)
	}

	if utf8.RuneCountInString(info.Repo) > 100 {
		return fmt.Errorf("GitHub repository name is too long: %s", info.Repo)
	}

	// Another low limit but our environments are usually short names.
	if utf8.RuneCountInString(info.Env) > 30 {
		return fmt.Errorf("environment name is too long: %s", info.Env)
	}

	return nil
}

func GetWorkflowInfoFromLabels(ctx context.Context, req types.AccessRequest) (GitHubWorkflowInfo, error) {
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
		return GitHubWorkflowInfo{}, newMissingLabelError(req, missingLabels)
	}

	runIDInt, err := strconv.Atoi(runID)
	if err != nil {
		return GitHubWorkflowInfo{}, fmt.Errorf("parsing workflow run ID: %w", err)
	}

	return GitHubWorkflowInfo{
		Org:           org,
		Repo:          repo,
		Env:           env,
		WorkflowRunID: int64(runIDInt),
	}, nil
}

func newMissingLabelError(req types.AccessRequest, labelKeys []string) *MissingLabelError {
	return &MissingLabelError{
		RequestID: req.GetName(),
		LabelKeys: labelKeys,
	}
}
