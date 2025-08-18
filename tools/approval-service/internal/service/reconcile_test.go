/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitingWorkflowReconciler(t *testing.T) {

	t.Run("Waiting Workflows", func(t *testing.T) {
		workflowA := int64(12345)
		workflowB := int64(67890)
		workflowC := int64(54321)

		tt := []struct {
			name                   string
			waitingWorkflows       []int64
			accessRequestsState    map[int64]types.RequestState
			expectHandledWorkflows []int64
			expectHandledRequests  []int64
		}{
			{
				name: "With pending Access Requests",
				// All workflows are waiting, and we have pending Access Requests for all of them.
				// This is the most common case where we have workflows that are waiting for approval.
				waitingWorkflows: []int64{workflowA, workflowB, workflowC},
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
				// Some workflows are waiting, but we have Access Requests that are already approved or denied.
				// In this case, the Access Requests weren't handled successfully previously, so we will handle them now.
				waitingWorkflows: []int64{workflowA, workflowB, workflowC},
				accessRequestsState: map[int64]types.RequestState{
					workflowA: types.RequestState_APPROVED,
					workflowB: types.RequestState_DENIED,
					workflowC: types.RequestState_PENDING,
				},
				expectHandledWorkflows: []int64{},
				expectHandledRequests:  []int64{workflowA, workflowB},
			},
			{
				name: "With no Access Requests",
				// All workflows are waiting, but we have no Access Requests for them.
				// In this case, the Deployment Event was not handled successfully previously, so we will handle them now.
				waitingWorkflows:       []int64{workflowA, workflowB, workflowC},
				accessRequestsState:    map[int64]types.RequestState{},
				expectHandledWorkflows: []int64{workflowA, workflowB, workflowC},
				expectHandledRequests:  []int64{},
			},
			{
				name: "No waiting workflows",
				// No workflows are waiting, so we don't expect anything to be handled.
				waitingWorkflows: []int64{},
				accessRequestsState: map[int64]types.RequestState{
					workflowA: types.RequestState_APPROVED,
					workflowB: types.RequestState_DENIED,
					workflowC: types.RequestState_PENDING,
				},
				expectHandledWorkflows: []int64{},
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
					require.NoError(t, newReq.SetState(state))

					err = SetWorkflowLabels(newReq, GithubWorkflowLabels{
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

				svc, err := NewReleaseService(
					config.Root{},
					newFakeTeleportClient(reqs),
					newFakeGitHubClient(tc.waitingWorkflows...),
				)

				require.NoError(t, err, "Failed to create ReleaseService")

				deploymentRules, accessRequests, err := svc.findReconciliationWork(ctx)
				assert.NoError(t, err, "Failed to find reconciliation work")

				for _, rule := range deploymentRules {
					if _, ok := checkHandledWorkflows[rule.WorkflowID]; ok {
						checkHandledWorkflows[rule.WorkflowID] = true
					}
				}
				for _, req := range accessRequests {
					githubLabels, err := GetWorkflowLabels(req)
					require.NoError(t, err, "Failed to get workflow labels from access request")
					if _, ok := checkHandledRequests[githubLabels.WorkflowRunID]; ok {
						checkHandledRequests[githubLabels.WorkflowRunID] = true
					}
				}

				for runID, handled := range checkHandledWorkflows {
					assert.True(t, handled, "Expected workflow %d to be handled but it was not", runID)
				}
				for runID, handled := range checkHandledRequests {
					assert.True(t, handled, "Expected access request for workflow %d to be handled but it was not", runID)
				}
			})
		}
	})
}
