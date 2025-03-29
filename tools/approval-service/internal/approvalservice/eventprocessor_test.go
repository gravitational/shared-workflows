package approvalservice

import (
	"context"
	"testing"

	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventProcessor(t *testing.T) {
	t.Run("Single event flow", func(t *testing.T) {
		p := newTestProcessor(t)
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		require.NoError(t, p.Setup(ctx))

		p.teleportClient = &fakeTeleportClient{
			// Creating an access request will callback to the processor with an approved state
			createAccessRequestV2Func: func(ctx context.Context, req types.AccessRequest) (types.AccessRequest, error) {
				if err := req.SetState(types.RequestState_APPROVED); err != nil {
					return req, err
				}
				err := p.HandleReview(ctx, req)
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
			updatePendingDeploymentFunc: func(ctx context.Context, org, repo string, runID int64, state github.PendingDeploymentApprovalState, opts *github.PendingDeploymentOpts) ([]github.Deployment, error) {
				assert.Equal(t, event.WorkflowID, runID)
				assert.Equal(t, event.Organization, org)
				assert.Equal(t, event.Repository, repo)
				assert.Equal(t, github.PendingDeploymentApprovalStateApproved, state)
				return nil, nil
			},
		}

		err := p.ProcessDeploymentReviewEvent(event, true)
		require.NoError(t, err)
	})
}

func newTestProcessor(t *testing.T) *processor {
	return newProcessor(
		config.Root{
			Teleport: config.Teleport{
				User:          "test-user",
				RoleToRequest: "gha-build-prod",
			},
			GitHubEvents: config.GitHubEvents{
				Validation: []config.Validation{
					{
						Org:          "gravitational",
						Repo:         "teleport",
						Environments: []string{"build/prod"},
					},
				},
			},
		},
		&fakeGitHubClient{
			updatePendingDeploymentFunc: func(ctx context.Context, org, repo string, runID int64, state github.PendingDeploymentApprovalState, opts *github.PendingDeploymentOpts) ([]github.Deployment, error) {
				t.Fatalf("updatePendingDeploymentFunc needs an implementation")
				return nil, nil
			},
			getEnvironmentFunc: func(ctx context.Context, org, repo, environment string) (github.Environment, error) {
				if org == "gravitational" && repo == "teleport" && environment == "build/prod" {
					return github.Environment{
						ID:   54321,
						Name: "build/prod",
						Org:  "gravitational",
						Repo: "teleport",
					}, nil
				}
				t.Fatalf("got unexpected environment %q", environment)
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
	updatePendingDeploymentFunc func(ctx context.Context, org, repo string, runID int64, state github.PendingDeploymentApprovalState, opts *github.PendingDeploymentOpts) ([]github.Deployment, error)
	getEnvironmentFunc          func(ctx context.Context, org, repo, environment string) (github.Environment, error)
}

func (f *fakeGitHubClient) UpdatePendingDeployment(ctx context.Context, org, repo string, runID int64, state github.PendingDeploymentApprovalState, opts *github.PendingDeploymentOpts) ([]github.Deployment, error) {
	return f.updatePendingDeploymentFunc(ctx, org, repo, runID, state, opts)
}

func (f *fakeGitHubClient) GetEnvironment(ctx context.Context, org, repo, environment string) (github.Environment, error) {
	return f.getEnvironmentFunc(ctx, org, repo, environment)
}
