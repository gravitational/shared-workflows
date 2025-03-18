package approvalservice

import (
	"context"
	"log/slog"
	"testing"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventProcessor(t *testing.T) {
	t.Run("Single event flow", func(t *testing.T) {
		p := newTestProcessor(t)

		p.teleportClient = &fakeTeleportClient{
			// Creating an access request will callback to the processor with an approved state
			createAccessRequestV2Func: func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
				if err := req.SetState(types.RequestState_APPROVED); err != nil {
					return req, err
				}
				err := p.HandleReview(context.TODO(), req)
				if err != nil {
					return req, err
				}
				return req, nil
			},
		}

		// Given an event, expect that we should approve the pending deployment review
		event := githubevents.DeploymentReviewEvent{
			WorkflowID:   int64(123456),
			Environment:  "build/prod",
			Requester:    "test-user",
			Organization: "gravitational",
			Repository:   "teleport",
		}

		p.githubClient = &fakeGitHubClient{
			updatePendingDeploymentFunc: func(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error) {
				assert.Equal(t, event.WorkflowID, info.RunID)
				assert.Equal(t, event.Organization, info.Org)
				assert.Equal(t, event.Repository, info.Repo)
				assert.Equal(t, github.PendingDeploymentApprovalStateApproved, info.State)
				return nil, nil
			},
		}

		err := p.ProcessDeploymentReviewEvent(event, true)
		require.NoError(t, err)
	})
}

func newTestProcessor(t *testing.T) *processor {
	return &processor{
		TeleportUser: "test-user",
		TeleportRole: "test-role",
		teleportClient: &fakeTeleportClient{
			createAccessRequestV2Func: func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
				t.Fatalf("createAccessRqeuestV2Func needs an implementation")
				return nil, nil
			},
		},
		githubClient: &fakeGitHubClient{
			updatePendingDeploymentFunc: func(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error) {
				t.Fatalf("updatePendingDeploymentFunc needs an implementation")
				return nil, nil
			},
		},
		log: slog.Default(),
	}
}

type fakeTeleportClient struct {
	createAccessRequestV2Func func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error)
}

func (f *fakeTeleportClient) CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
	return f.createAccessRequestV2Func(ctx, req)
}

type fakeGitHubClient struct {
	updatePendingDeploymentFunc func(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error)
}

func (f *fakeGitHubClient) UpdatePendingDeployment(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error) {
	return f.updatePendingDeploymentFunc(ctx, info)
}
