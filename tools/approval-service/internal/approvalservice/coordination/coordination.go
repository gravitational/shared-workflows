package coordination

import (
	"context"
	"errors"
)

type Coordinator interface {
	// LeaseGitHubWorkflow leases a GitHub workflow run for the given org, repo, and run ID.
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	//
	// Rate limiting is applied to the workflow ID to prevent excessive lease acquisition attempts per service.
	// The rate limit is set to 1 request per workflow lease duration.
	// If the rate limit is exceeded, an error is returned.
	LeaseGitHubWorkflow(ctx context.Context, org, repo string, runID int64) error

	// LeaseAccessRequest leases an access request for the given ID.
	// This prevents multiple services from processing the same access request concurrently.
	//
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	LeaseAccessRequest(ctx context.Context, id string) error
}

type coordinator struct{}

func NewCoordinator() (Coordinator, error) {
	return &coordinator{}, nil
}

func (c *coordinator) LeaseGitHubWorkflow(ctx context.Context, org, repo string, runID int64) error {
	return errors.New("not implemented")
}

func (c *coordinator) LeaseAccessRequest(ctx context.Context, id string) error {
	return errors.New("not implemented")
}
