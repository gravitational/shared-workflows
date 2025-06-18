package githubreconcile

import (
	"context"
	"time"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/accessrequest"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/sources/githubevents"
	"github.com/gravitational/teleport/api/types"
)

// TODO: Implement the reconciler in a second pass after first code review.

// Reconciler is a service that reconciles the state of GitHub deployment protection rules with the state of Teleport access requests.
// Events are ephemeral and failures to process results in a loss of data. This serves as a redundancy for such cases.
// To recover from this, the reconciler will periodically check the state of the deployment protection rules and the state of the access requests.
// If it detects a mismatch, it will fire an event to update the state of the access request.
type Reconciler struct {
	deployReviewEventProcessor githubevents.GitHubEventProcessor
	reviewHandler              accessrequest.ReviewHandler
}

// Small interface to allow for easier testing of the Teleport client.
// This is a subset of the teleport.Client interface that we need for our purposes.
// It is not intended to be a complete representation of the Teleport API or the teleport.Client implementation.
type teleClient interface {
	GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error)
}

func NewReconciler(ghClient github.Client, teleClient teleClient, org, repo string) *Reconciler {
	return &Reconciler{}
}

func (r *Reconciler) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {

	// GitHub is the source of truth for all state changes.
	// We only need to consider unresolved GitHub deployment protection rules. (i.e. pending approvals)
	// For each pending deployment protection rule, we need to check if there is a corresponding access request in Teleport.
	// If there is, we need to check if the state of the access request matches the state of the deployment protection rule.
	// If there is no corresponding access request, we need to "refire" the event and handle with deployReviewEventProcessor
	// If there is a corresponding access request that is not pending, we "refire" the event and handle with reviewHandler
}
