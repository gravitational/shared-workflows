package approvalservice

import (
	"context"
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
		require.NoError(t, p.Setup())

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
				assert.Equal(t, github.PendingDeploymentApprovalState_APPROVED, info.State)
				assert.Equal(t, int64(54321), info.EnvIDs[0])
				return nil, nil
			},
		}

		err := p.ProcessDeploymentReviewEvent(event, true)
		require.NoError(t, err)
	})
}

func newTestProcessor(t *testing.T) *processor {
	return newProcessor(
		Config{
			Teleport: TeleportConfig{
				User:          "test-user",
				RoleToRequest: "gha-build-prod",
			},
			GitHubEvents: githubevents.Config{
				Validation: []githubevents.ValidationConfig{
					{
						Org:          "gravitational",
						Repo:         "teleport",
						Environments: []string{"build/prod"},
					},
				},
			},
		},
		&fakeGitHubClient{
			updatePendingDeploymentFunc: func(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error) {
				t.Fatalf("updatePendingDeploymentFunc needs an implementation")
				return nil, nil
			},
			getEnvironmentFunc: func(ctx context.Context, info github.GetEnvironmentInfo) (github.Environment, error) {
				if info.Environment == "build/prod" {
					return github.Environment{
						ID:   54321,
						Name: "build/prod",
						Org:  "gravitational",
						Repo: "teleport",
					}, nil
				}
				t.Fatalf("got unexpected environment %q", info.Environment)
				return github.Environment{}, nil
			},
		},
		&fakeTeleportClient{
			createAccessRequestV2Func: func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
				t.Fatalf("createAccessRqeuestV2Func needs an implementation")
				return nil, nil
			},
		},
	)
}

type fakeTeleportClient struct {
	createAccessRequestV2Func func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error)
}

func (f *fakeTeleportClient) CreateAccessRequestV2(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
	return f.createAccessRequestV2Func(ctx, req)
}

type fakeGitHubClient struct {
	updatePendingDeploymentFunc func(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error)
	getEnvironmentFunc          func(ctx context.Context, info github.GetEnvironmentInfo) (github.Environment, error)
}

func (f *fakeGitHubClient) UpdatePendingDeployment(ctx context.Context, info github.PendingDeploymentInfo) ([]github.Deployment, error) {
	return f.updatePendingDeploymentFunc(ctx, info)
}

func (f *fakeGitHubClient) GetEnvironment(ctx context.Context, info github.GetEnvironmentInfo) (github.Environment, error) {
	return f.getEnvironmentFunc(ctx, info)
}
