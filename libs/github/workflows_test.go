package github

import (
	"net/http"
	"testing"
)

func TestWorkflowDispatch(t *testing.T) {
	tests := []struct {
		name           string
		workflowName   string
		ref            string
		inputs         map[string]any
		mockResponse   string
		mockStatus     int
		wantError      bool
		wantWorkflowID int64
		wantHTMLURL    string
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
			wantHTMLURL:    "https://github.com/gravitational/test-workflows/actions/runs/22638542196",
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
				if result.HTMLURL != tt.wantHTMLURL {
					t.Errorf("expected HTML URL %s, got %s", tt.wantHTMLURL, result.HTMLURL)
				}
			}
		})
	}
}

func TestRerunWorkflowFailedJobs(t *testing.T) {
	tests := []struct {
		name         string
		workflowRunID int64
		mockStatus   int
		wantError    bool
	}{
		{
			name:         "happy path",
			workflowRunID: 22638542196,
			mockStatus:   http.StatusOK,
			wantError:    false,
		},
		{
			name:         "API error",
			workflowRunID: 22638542196,
			mockStatus:   http.StatusInternalServerError,
			wantError:    true,
		},
		{
			name:         "missing workflow run ID",
			workflowRunID: 0,
			mockStatus:   http.StatusInternalServerError,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}
				w.WriteHeader(tt.mockStatus)
			})

			client, closer := newFakeClient(mux)
			t.Cleanup(closer)

			ctx := t.Context()
			err := client.RerunWorkflowFailedJobs(ctx, "gravitational", "example-repo", tt.workflowRunID)

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}
