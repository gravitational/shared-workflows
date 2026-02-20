package github

import (
	"errors"
	"net/http"
	"testing"
)

func TestFindWorkflowRunIDByUniqueStepName(t *testing.T) {
	// correlationID is a static value that matches the step name in testdata/list-jobs-with-correlation-id.json
	const correlationID = "c658ef64-1ea1-4802-ac75-92a43e4d8012"
	const workflowName = "test-workflow.yml"

	tests := []struct {
		name                  string
		listWorkflowsResponse string
		listJobsResponse      string
		listWorkflowsStatus   int
		listJobsStatus        int
		wantError             bool
		wantWorkflowNotFound  bool
	}{
		{
			name:                  "happy path - correlation ID found in step",
			listWorkflowsResponse: "list-workflows.json",
			listJobsResponse:      "list-jobs-with-correlation-id.json",
			listWorkflowsStatus:   http.StatusOK,
			listJobsStatus:        http.StatusOK,
			wantError:             false,
			wantWorkflowNotFound:  false,
		},
		{
			name:                  "correlation ID not found yet - step name missing",
			listWorkflowsResponse: "list-workflows.json",
			listJobsResponse:      "list-jobs-with-correlation-id-missing.json",
			listWorkflowsStatus:   http.StatusOK,
			listJobsStatus:        http.StatusOK,
			wantError:             true,
			wantWorkflowNotFound:  true,
		},
		{
			name:                  "list workflows returns 404",
			listWorkflowsResponse: "",
			listJobsResponse:      "list-jobs-with-correlation-id.json",
			listWorkflowsStatus:   http.StatusNotFound,
			listJobsStatus:        http.StatusOK,
			wantError:             true,
			wantWorkflowNotFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", func(w http.ResponseWriter, r *http.Request) {
				if tt.listWorkflowsStatus != http.StatusOK {
					w.WriteHeader(tt.listWorkflowsStatus)
					return
				}
				respondWithJSONTestdata(w, tt.listWorkflowsResponse)
			})

			mux.HandleFunc("/repos/{owner}/{repo}/actions/runs/{run_id}/jobs", func(w http.ResponseWriter, r *http.Request) {
				if tt.listJobsStatus != http.StatusOK {
					w.WriteHeader(tt.listJobsStatus)
					return
				}
				respondWithJSONTestdata(w, tt.listJobsResponse)
			})

			client, closer := newFakeClient(mux)
			t.Cleanup(closer)

			ctx := t.Context()
			result, err := client.FindWorkflowRunByCorrelatedStepName(ctx, "gravitational", "example-repo", FindWorkflowRunByCorrelatedStepNameParams{
				WorkflowName:  workflowName,
				CorrelationID: correlationID,
			})

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.wantWorkflowNotFound {
					var notFoundErr *WorkflowRunNotFoundError
					if !errors.As(err, &notFoundErr) {
						t.Errorf("expected WorkflowRunNotFoundError, got %T: %v", err, err)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				// Verify we got a valid result
				if result.WorkflowID == 0 {
					t.Error("expected non-zero workflow run ID")
				}
			}
		})
	}
}
