package store

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gravitational/teleport/api/types"
)

// Repository is used for managing external data related to Access Requests and their lifecycle.
type Repository struct {
	// GitHubService is responsible for storing and retrieving GitHub workflow information.
	GitHub GitHubService

	// ProcessorService is responsible for managing high-level information about event processing.
	ProcessorService ProcessorService

	githubOpts *githubOpts
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

// NewRepository creates a new Repository instance with the provided GitHubService.
func NewRepository(opts ...RepositoryOpt) (*Repository, error) {
	ghOpts := &githubOpts{
		log: slog.Default(),
	}
	repo := &Repository{
		GitHub:           &githubStore{githubOpts: ghOpts},
		ProcessorService: &processorService{},
		githubOpts:       ghOpts,
	}

	for _, o := range opts {
		if err := o(repo); err != nil {
			return nil, fmt.Errorf("applying repository option: %w", err)
		}
	}

	return repo, nil
}

// RepositoryOpt is a functional option type for configuring the Repository.
type RepositoryOpt func(*Repository) error

var WithGitHubLogger = func(logger *slog.Logger) RepositoryOpt {
	return func(r *Repository) error {
		if logger == nil {
			return errors.New("GitHub logger cannot be nil")
		}
		r.githubOpts.log = logger
		return nil
	}
}

func newMissingLabelError(req types.AccessRequest, labelKeys []string) *MissingLabelError {
	return &MissingLabelError{
		RequestID: req.GetName(),
		LabelKeys: labelKeys,
	}
}
