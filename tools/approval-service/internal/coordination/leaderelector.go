package coordination

import (
	"context"
	"errors"
)

// LeaderElector provides methods for acquiring leases for tasks that require exclusive access.
type LeaderElector interface {
	// LeaseHandleDeploymentReviewEventReceived leases a GitHub workflow run for the given org, repo, and run ID.
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	//
	// Rate limiting is applied to the workflow ID to prevent excessive lease acquisition attempts per service.
	// The rate limit is set to 1 request per workflow lease duration.
	// If the rate limit is exceeded, an error is returned.
	LeaseHandleDeploymentReviewEventReceived(ctx context.Context, org, repo string, runID int64) error

	// LeaseHandleAccessRequestReviewed leases an access request for the given ID.
	// This prevents multiple services from processing the same access request concurrently.
	//
	// This will block until the lease is acquired or it is determined that the lease cannot be acquired (e.g. already leased).
	LeaseHandleAccessRequestReviewed(ctx context.Context, id string) error
}

type leaderelector struct{}

func NewLeaderElector() (LeaderElector, error) {
	return &leaderelector{}, nil
}

func (c *leaderelector) LeaseHandleDeploymentReviewEventReceived(ctx context.Context, org, repo string, runID int64) error {
	return errors.New("not implemented")
}

func (c *leaderelector) LeaseHandleAccessRequestReviewed(ctx context.Context, id string) error {
	return errors.New("not implemented")
}
