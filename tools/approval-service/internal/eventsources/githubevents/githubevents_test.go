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

package githubevents

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSource(t *testing.T) {
	tests := []struct {
		name     string
		proc     GitHubEventProcessor
		testFile string
		// eventType is to set the X-GitHub-Event header
		eventType string
		// a non 200 response is considered an error
		expectErr bool
	}{
		{
			name: "Valid Deployment Review Event",
			proc: &fakeGitHubEventProcessor{
				ProcessDeploymentReviewEventFunc: func(ctx context.Context, event DeploymentReviewEvent) error {
					assert.Equal(t, "gravitational", event.Organization)
					assert.Equal(t, "test-repo", event.Repository)
					assert.Equal(t, "build/prod", event.Environment)
					assert.Equal(t, "gravitational-member", event.Requester)
					assert.Equal(t, int64(14988928371), event.WorkflowID)
					return nil
				},
			},
			testFile:  "testdata/deployment_review_event.json",
			eventType: "deployment_review",
		},
		{
			name: "Valid Workflow Dispatch Event",
			proc: &fakeGitHubEventProcessor{
				ProcessWorkflowDispatchEventFunc: func(ctx context.Context, event WorkflowDispatchEvent) error {
					assert.Equal(t, "gravitational", event.Organization)
					assert.Equal(t, "test-repo", event.Repository)
					assert.Equal(t, "gravitational-member", event.Requester)
					assert.Equal(t, map[string]string{"input1": "value1"}, event.Inputs)
					return nil
				},
			},
			testFile:  "testdata/workflow_dispatch_event.json",
			eventType: "workflow_dispatch",
			expectErr: true, // Not currently implemented
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sanity check to ensure that the source can be created and the handler is set up correctly.
			src, err := NewSource(
				config.GitHubSource{},
				tt.proc,
				withoutValidation(),
			)
			require.NoError(t, err)
			srv := httptest.NewServer(src.handler)
			t.Cleanup(srv.Close)

			f, err := os.Open(tt.testFile)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", srv.URL, f)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", tt.eventType)

			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
			if tt.expectErr {
				assert.NotEqual(t, http.StatusOK, resp.StatusCode)
			} else {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
			}
		})
	}
}

// disables payload validation for testing purposes
func withoutValidation() SourceOpt {
	return func(s *Source) error {
		s.payloadValidationDisabled = true
		return nil
	}
}

type fakeGitHubEventProcessor struct {
	ProcessDeploymentReviewEventFunc func(ctx context.Context, event DeploymentReviewEvent) error
	ProcessWorkflowDispatchEventFunc func(ctx context.Context, event WorkflowDispatchEvent) error
}

func (f *fakeGitHubEventProcessor) ProcessDeploymentReviewEvent(ctx context.Context, event DeploymentReviewEvent) error {
	return f.ProcessDeploymentReviewEventFunc(ctx, event)
}

func (f *fakeGitHubEventProcessor) ProcessWorkflowDispatchEvent(ctx context.Context, event WorkflowDispatchEvent) error {
	return f.ProcessWorkflowDispatchEventFunc(ctx, event)
}

// FuzzHeaders checks the handling of arbitrary headers in the GitHub webhook handler.
// Since this is a public endpoint, it should gracefully handle unexpected headers without panicking.
// It reads a sample event from a file and then fuzzes the handler with random headers.
func FuzzHeaders(f *testing.F) {
	eventFile, err := os.Open("testdata/deployment_review_event.json")
	require.NoError(f, err)
	event, err := io.ReadAll(eventFile)
	require.NoError(f, err)
	require.NoError(f, eventFile.Close())

	src, err := NewSource(
		config.GitHubSource{
			Secret: "test-secret",
		},
		&fakeGitHubEventProcessor{
			ProcessDeploymentReviewEventFunc: func(ctx context.Context, event DeploymentReviewEvent) error {
				// This should not be called during fuzzing
				f.Fatal("unexpected call to ProcessDeploymentReviewEvent")
				return nil
			},
		},
	)
	require.NoError(f, err)

	f.Add("sha256=fakehash", "application/json", "deployment_review")
	f.Fuzz(func(t *testing.T, hash, contentType, eventType string) {
		buff := bytes.NewBuffer(event)
		req := httptest.NewRequest("POST", "/webhook", buff)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("X-GitHub-Event", eventType)
		req.Header.Set("X-Hub-Signature-256", hash)
		w := httptest.NewRecorder()

		src.Handler().ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Result().StatusCode)
	})
}

// FuzzDeploymentReview is a fuzz test for the Deployment Review event handler.
// It reads a sample event from a file and then fuzzes the handler with random data.
// This is useful to ensure that the handler can handle unexpected or malformed input gracefully. (e.g. no panics)
func FuzzDeploymentReview(f *testing.F) {
	eventFile, err := os.Open("testdata/deployment_review_event.json")
	require.NoError(f, err)
	event, err := io.ReadAll(eventFile)
	require.NoError(f, err)
	require.NoError(f, eventFile.Close())

	src, err := NewSource(
		config.GitHubSource{},
		&fakeGitHubEventProcessor{
			ProcessDeploymentReviewEventFunc: func(ctx context.Context, event DeploymentReviewEvent) error {
				// noop
				return nil
			},
		},
		withoutValidation(),
	)
	require.NoError(f, err)

	f.Add(event)
	f.Fuzz(func(t *testing.T, data []byte) {
		buff := bytes.NewBuffer(data)
		req := httptest.NewRequest("POST", "/webhook", buff)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "deployment_review")
		w := httptest.NewRecorder()

		src.Handler().ServeHTTP(w, req)
	})
}
