package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/libs/github"
	"github.com/gravitational/teleport/api/types"
)

func TestReleaseService(t *testing.T) {
	t.Run("initialize with valid config", func(t *testing.T) {
	})
}

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
}

func (f *fakeGitHubClient) GetWorkflowRunInfo(ctx context.Context, org, repo string, runID int64) (github.WorkflowRunInfo, error) {
	return github.WorkflowRunInfo{}, nil
}
func (f *fakeGitHubClient) ReviewDeploymentProtectionRule(ctx context.Context, org, repo string, info github.ReviewDeploymentProtectionRuleInfo) error {
	return nil
}
