package github

import (
	"net/http"
	"testing"
)

<<<<<<< HEAD
func TestWorkflowDispatch(t *testing.T) {
=======
func TestFindWorkflowRunIDByUniqueStepName(t *testing.T) {
	// correlationID is a static value that matches the step name in testdata/list-jobs-with-correlation-id.json
	const correlationID = "c658ef64-1ea1-4802-ac75-92a43e4d8012"
	const workflowName = "test-workflow.yml"

>>>>>>> 832d8c8 (Formatting)
	tests := []struct {
		name           string
		workflowName   string
		ref            string
		inputs         map[string]any
		mockResponse   string
		mockStatus     int
		wantError      bool
		wantWorkflowID int64
	}{
		{
			name:           "happy path",
			workflowName:   "test-workflow.yml",
			ref:            "main",
			inputs:         map[string]any{"dog": "cat"},
			mockResponse:   "dispatch-workflow.json",
			mockStatus:     http.StatusOK,
			wantError:      false,
			wantWorkflowID: 22638542196,
		},
		{
			name:         "missing workflow name",
			workflowName: "",
			ref:          "main",
			wantError:    true,
		},
		{
			name:         "missing ref",
			workflowName: "test-workflow.yml",
			ref:          "",
			wantError:    true,
		},
		{
			name:         "API error",
			workflowName: "test-workflow.yml",
			ref:          "main",
			mockStatus:   http.StatusInternalServerError,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}
				if tt.mockStatus != http.StatusOK {
					w.WriteHeader(tt.mockStatus)
					return
				}
				respondWithJSONTestdata(w, tt.mockResponse)
			})

			client, closer := newFakeClient(mux)
			t.Cleanup(closer)

			ctx := t.Context()
			result, err := client.WorkflowDispatch(ctx, "gravitational", "example-repo", WorkflowDispatchRequest{
				WorkflowName: tt.workflowName,
				Ref:          tt.ref,
				Inputs:       tt.inputs,
			})

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				if result.WorkflowRunID != tt.wantWorkflowID {
					t.Errorf("expected workflow run ID %d, got %d", tt.wantWorkflowID, result.WorkflowRunID)
				}
			}
		})
	}
}
