package githubevents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
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
			defer f.Close()

			req, err := http.NewRequest("POST", srv.URL, f)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", tt.eventType)

			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
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
