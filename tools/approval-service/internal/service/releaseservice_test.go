package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/eventsources/githubevents"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReleaseService(t *testing.T) {
	initService := func(teleClient *fakeTeleportClient, ghClient *fakeGitHubClient) *ReleaseService {
		svc, err := NewReleaseService(
			config.Root{
				ApprovalService: config.ApprovalService{
					Teleport: config.Teleport{
						User: "test-tele-user",
					},
				},
				EventSources: config.EventSources{
					GitHub: config.GitHubSource{
						Org:  "test-org",
						Repo: "test-repo",
						Environments: []config.GitHubEnvironment{
							{
								Name:         "build-staging",
								TeleportRole: "gha-env-build-staging",
							},
						},
					},
				},
			},
			teleClient,
			ghClient,
			WithLogger(slog.Default()),
		)
		require.NoError(t, err)
		return svc
	}

	t.Run("New GitHub Workflow = New Access Request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		teleClient := newFakeTeleportClient(nil)
		ghClient := newFakeGitHubClient()
		svc := initService(teleClient, ghClient)

		go func() {
			err := svc.Run(ctx)
			assert.ErrorIs(t, err, context.Canceled, "Expected Run to return context.Canceled when context is canceled")
		}()

		// Simulate receiving a deployment review event
		// This should create three Access Requests all requesting the "gha-env-build-staging" role
		xWorkflowID := int64(12345)
		yWorkflowID := int64(67890)
		zWorkflowID := int64(54321)

		for _, id := range []int64{xWorkflowID, yWorkflowID, zWorkflowID} {
			// Simulate multiple deployment review events for the same workflow ID
			for range 3 {
				svc.onDeploymentReviewEventReceived(ctx, githubevents.DeploymentReviewEvent{
					WorkflowID:   id,
					Environment:  "build-staging",
					Requester:    "test-gh-user",
					Organization: "test-org",
					Repository:   "test-repo",
				})
			}
		}

		// Verify that the Access Requests were created
		reqs, err := teleClient.GetAccessRequests(ctx, types.AccessRequestFilter{})
		require.NoError(t, err)
		assert.Len(t, reqs, 3) // We expect three Access Requests, one for each workflow ID
		for _, req := range reqs {
			info, err := GetWorkflowLabels(req)
			require.NoError(t, err, "Expected to get workflow labels without error")

			assert.Equal(t, "gha-env-build-staging", req.GetRoles()[0], "Expected role to be gha-env-build-staging")
			assert.Equal(t, "test-tele-user", req.GetUser(), "Expected requester to be test-tele-user")
			assert.Equal(t, "test-org", info.Org, "Expected organization to be test-org")
			assert.Equal(t, "test-repo", info.Repo, "Expected repository to be test-repo")
			assert.Equal(t, "build-staging", info.Env, "Expected environment to be build-staging")
			assert.Contains(t, []int64{xWorkflowID, yWorkflowID, zWorkflowID}, info.WorkflowRunID, "Expected workflow run ID to match one of the test IDs")
		}
	})

	t.Run("Handle Access Request Reviewed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		teleClient := newFakeTeleportClient(nil)
		ghClient := newFakeGitHubClient()
		svc := initService(teleClient, ghClient)

		go func() {
			err := svc.Run(ctx)
			assert.ErrorIs(t, err, context.Canceled, "Expected Run to return context.Canceled when context is canceled")
		}()

		tt := []struct {
			name       string
			workflowID int64
			status     types.RequestState
		}{
			{
				name:       "Approve Access Request",
				workflowID: 12345,
				status:     types.RequestState_APPROVED,
			},
			{
				name:       "Deny Access Request",
				workflowID: 67890,
				status:     types.RequestState_DENIED,
			},
		}

		for _, tc := range tt {
			t.Run(tc.name, func(t *testing.T) {
				// Create a new Access Request for the workflow ID
				req, err := types.NewAccessRequest(uuid.NewString(), "test-tele-user", "gha-env-build-staging")
				require.NoError(t, err, "Expected to create new Access Request without error")
				require.NoError(t, req.SetState(tc.status))

				err = SetWorkflowLabels(req, GithubWorkflowLabels{
					WorkflowRunID: tc.workflowID,
					Org:           "test-org",
					Repo:          "test-repo",
					Env:           "build-staging",
				})
				require.NoError(t, err, "Expected to set workflow labels without error")

				svc.onAccessRequestReviewed(ctx, req)

				approved, err := ghClient.isRunApproved("test-org", "test-repo", tc.workflowID)
				assert.NoError(t, err, "Expected to check if workflow run is approved without error")
				assert.Equal(t, tc.status == types.RequestState_APPROVED, approved, "Expected workflow run approval status to match")
			})
		}
	})

	t.Run("New Pending Deployment Review with existing Access Request", func(t *testing.T) {

		// This test simulates a scenario where a deployment review event is received for a workflow run that already has an Access Request created.
		// This can happen in the following scenario:
		// 1. A workflow is retried after cancellation or failure.
		// 2. A downstream job in the same workflow is started that requires the same environment approval.
		tt := []struct {
			name                      string
			initialAccessRequestState types.RequestState
			workflowID                int64
		}{
			{
				name:                      "Initial Access Request Approved",
				initialAccessRequestState: types.RequestState_APPROVED,
				workflowID:                67890,
			},
			{
				name:                      "Initial Access Request Denied",
				initialAccessRequestState: types.RequestState_DENIED,
				workflowID:                54321,
			},
		}

		for _, tc := range tt {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				req, err := types.NewAccessRequest(uuid.NewString(), "test-tele-user", "gha-env-build-staging")
				require.NoError(t, err, "Expected to create new Access Request without error")
				require.NoError(t, req.SetState(tc.initialAccessRequestState), "Expected to set initial access request state without error")
				err = SetWorkflowLabels(req, GithubWorkflowLabels{
					WorkflowRunID: tc.workflowID,
					Org:           "test-org",
					Repo:          "test-repo",
					Env:           "build-staging",
				})
				require.NoError(t, err, "Expected to set workflow labels without error")

				teleClient := newFakeTeleportClient([]types.AccessRequest{req})
				ghClient := newFakeGitHubClient()
				svc := initService(teleClient, ghClient)
				go func() {
					err := svc.Run(ctx)
					assert.ErrorIs(t, err, context.Canceled, "Expected Run to return context.Canceled when context is canceled")
				}()

				// Simulate receiving a Deployment Review Event
				svc.onDeploymentReviewEventReceived(ctx, githubevents.DeploymentReviewEvent{
					WorkflowID:   tc.workflowID,
					Environment:  "build-staging",
					Requester:    "test-gh-user",
					Organization: "test-org",
					Repository:   "test-repo",
				})

				// Check that the GitHub client was called to review the deployment protection rule
				// It is handled asynchronously, so we need to wait for it to complete
				waitFunc := func() bool {
					approved, err := ghClient.isRunApproved("test-org", "test-repo", tc.workflowID)
					require.NoError(t, err, "Expected to check if workflow run is approved without error")
					return approved == (tc.initialAccessRequestState == types.RequestState_APPROVED)
				}
				assert.Eventually(t, waitFunc, 200*time.Millisecond, 20*time.Millisecond)
			})
		}
	})
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

func (f *fakeTeleportClient) CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
	if req == nil {
		return nil, errors.New("access request cannot be nil")
	}

	// Simulate creating a new Access Request by appending it to the in-memory slice
	// Create a copy of the request to avoid modifying the original.
	// The name is replaced with a new UUID to simulate the actual behavior of Teleport where the name is not preserved.
	reqCopy := req.Copy()
	reqCopy.SetName(uuid.NewString())
	f.reqs = append(f.reqs, reqCopy)
	return reqCopy, nil
}

type fakeGitHubClient struct {
	approvedState map[string]bool // to track if a workflow run is approved
}

func newFakeGitHubClient() *fakeGitHubClient {
	return &fakeGitHubClient{
		approvedState: make(map[string]bool),
	}
}

func (f *fakeGitHubClient) GetWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (github.WorkflowRunInfo, error) {

	return github.WorkflowRunInfo{}, nil
}
func (f *fakeGitHubClient) ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, info github.ReviewDeploymentProtectionRuleInfo) error {
	f.approvedState[fmt.Sprintf("%s/%s/%d", org, repo, info.RunID)] = info.State == github.PendingDeploymentApprovalStateApproved
	return nil
}

// helpful for testing purposes to check if a workflow run is approved or not
func (f *fakeGitHubClient) isRunApproved(org, repo string, runID int64) (bool, error) {
	val, ok := f.approvedState[fmt.Sprintf("%s/%s/%d", org, repo, runID)]
	if !ok {
		return false, fmt.Errorf("workflow run %d not found for org %s and repo %s", runID, org, repo)
	}
	return val, nil
}
