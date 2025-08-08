package githubreconcile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/service"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRepo         string = "test-repo"
	testOrg          string = "test-org"
	testEnv          string = "test-env"
	testTeleportUser string = "test-teleport-user"
	testTeleportRole string = "test-teleport-role"
)

func TestWaitingWorkflowReconciler(t *testing.T) {

	t.Run("Waiting Workflows", func(t *testing.T) {
		workflowA := int64(12345)
		workflowB := int64(67890)
		workflowC := int64(54321)

		tt := []struct {
			name                   string
			accessRequestsState    map[int64]types.RequestState
			expectHandledWorkflows []int64
			expectHandledRequests  []int64
		}{
			{
				name: "With pending Access Requests",
				accessRequestsState: map[int64]types.RequestState{
					workflowA: types.RequestState_PENDING,
					workflowB: types.RequestState_PENDING,
					workflowC: types.RequestState_PENDING,
				},
				expectHandledWorkflows: []int64{},
				expectHandledRequests:  []int64{},
			},
			{
				name: "With approved/denied Access Requests",
				accessRequestsState: map[int64]types.RequestState{
					workflowA: types.RequestState_APPROVED,
					workflowB: types.RequestState_DENIED,
					workflowC: types.RequestState_PENDING,
				},
				expectHandledWorkflows: []int64{},
				expectHandledRequests:  []int64{workflowA, workflowB},
			},
			{
				name:                   "With no Access Requests",
				accessRequestsState:    map[int64]types.RequestState{},
				expectHandledWorkflows: []int64{workflowA, workflowB, workflowC},
				expectHandledRequests:  []int64{},
			},
		}

		for _, tc := range tt {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)

				reqs := make([]types.AccessRequest, 0, len(tc.accessRequestsState))
				for runID, state := range tc.accessRequestsState {
					newReq, err := types.NewAccessRequest(uuid.NewString(), testTeleportUser, testTeleportRole)
					require.NoError(t, err, "Failed to create access request")
					newReq.SetState(state)

					err = service.SetWorkflowLabels(newReq, service.GithubWorkflowLabels{
						Org:           testOrg,
						Repo:          testRepo,
						Env:           testEnv,
						WorkflowRunID: runID,
					})
					require.NoError(t, err, "Failed to set workflow labels on access request")

					reqs = append(reqs, newReq)
				}

				checkHandledWorkflows := map[int64]bool{}
				for _, runID := range tc.expectHandledWorkflows {
					checkHandledWorkflows[runID] = false
				}

				checkHandledRequests := map[int64]bool{}
				for _, runID := range tc.expectHandledRequests {
					checkHandledRequests[runID] = false
				}

				reconciler, err := NewWaitingWorkflowReconciler(
					Config{
						Org:            testOrg,
						Repo:           testRepo,
						GitHubClient:   newFakeGitHubClient(workflowA, workflowB, workflowC),
						TeleportUser:   testTeleportUser,
						TeleportClient: newFakeTeleportClient(reqs),
						AccessRequestReviewHandler: fakeAccessRequestReviewHandler(func(ctx context.Context, req types.AccessRequest) error {
							labels, err := service.GetWorkflowLabels(req)
							require.NoError(t, err, "Failed to get workflow run ID from labels")
							if _, ok := checkHandledRequests[labels.WorkflowRunID]; !ok {
								t.Errorf("unexpected workflow run ID %d in access request: %v", labels.WorkflowRunID, req)
								return errors.New("test failure")
							}
							checkHandledRequests[labels.WorkflowRunID] = true
							return nil
						}),
						DeploymentReviewEventProcessor: fakeGitHubEventProcessor(func(ctx context.Context, event githubevents.DeploymentReviewEvent) error {
							checkHandledWorkflows[event.WorkflowID] = true
							if _, ok := checkHandledWorkflows[event.WorkflowID]; !ok {
								t.Errorf("unexpected workflow run ID %d in event: %v", event.WorkflowID, event)
								return errors.New("test failure")
							}
							checkHandledWorkflows[event.WorkflowID] = true
							return nil
						}),
					},
				)
				require.NoError(t, err, "Failed to create reconciler")

				go func() {
					err := reconciler.Run(ctx)
					assert.ErrorIs(t, err, context.Canceled, "should only exit with context cancellation")
				}()

				assert.NoError(t, reconciler.reconcile(ctx))
				for runID, handled := range checkHandledWorkflows {
					assert.True(t, handled, "workflow run %d was not handled", runID)
				}
				for runID, handled := range checkHandledRequests {
					assert.True(t, handled, "access request for workflow run %d was not handled", runID)
				}
			})
		}
	})
}

type fakeAccessRequestReviewHandler func(ctx context.Context, req types.AccessRequest) error

func (f fakeAccessRequestReviewHandler) HandleAccessRequestReviewed(ctx context.Context, req types.AccessRequest) error {
	return f(ctx, req)
}

type fakeGitHubEventProcessor func(ctx context.Context, event githubevents.DeploymentReviewEvent) error

func (f fakeGitHubEventProcessor) HandleDeploymentReviewEventReceived(ctx context.Context, event githubevents.DeploymentReviewEvent) error {
	return f(ctx, event)
}

// fakeTeleportClient is a stub implementation of the Teleport client for testing purposes.
// It stores Access Requests in memory and allows for basic operations like creating and retrieving Access Requests.
type fakeTeleportClient struct {
	// Store Access Requests in memory for testing
	reqs []types.AccessRequest
}

func newFakeTeleportClient(initialReqs []types.AccessRequest) *fakeTeleportClient {
	return &fakeTeleportClient{
		reqs: initialReqs,
	}
}

func (f *fakeTeleportClient) GetAccessRequests(ctx context.Context, filter types.AccessRequestFilter) ([]types.AccessRequest, error) {
	return f.reqs, nil
}

type fakeGitHubClient struct {
	workflowIDs []int64
}

func newFakeGitHubClient(workflowIDs ...int64) *fakeGitHubClient {
	return &fakeGitHubClient{
		workflowIDs: workflowIDs,
	}
}

func (f *fakeGitHubClient) ListWaitingWorkflowRuns(ctx context.Context, org, repo string) ([]github.WorkflowRunInfo, error) {
	var workflows []github.WorkflowRunInfo
	for _, id := range f.workflowIDs {
		workflows = append(workflows, github.WorkflowRunInfo{
			WorkflowID: id,
		})
	}
	return workflows, nil
}

func (f *fakeGitHubClient) GetPendingDeployments(ctx context.Context, org, repo string, runID int64) ([]github.PendingDeploymentInfo, error) {
	return []github.PendingDeploymentInfo{
		{
			Environment: testEnv,
		},
	}, nil
}
