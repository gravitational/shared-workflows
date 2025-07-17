package store

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gravitational/teleport/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubService(t *testing.T) {

	t.Run("StoreWorkflowRunInfo", func(t *testing.T) {
		storeWorkflowRunInfoTestFunc := func(t *testing.T, info GitHubWorkflowInfo, wantErr bool) {
			repo, err := NewRepository()
			require.NoError(t, err, "failed to create GitHub repository")

			req, err := types.NewAccessRequest(uuid.NewString(), "test-user", "test-role")
			require.NoError(t, err, "failed to create access request")

			err = repo.GitHub.StoreWorkflowInfo(context.Background(), req, info)
			if wantErr {
				assert.Error(t, err, "expected error but got none")
				return
			}
			assert.NoError(t, err, "unexpected error storing workflow info")
			// Verify that the info was stored correctly

			assert.Equal(t, info.Org, req.GetStaticLabels()[organizationLabel], "expected org to match")
			assert.Equal(t, info.Repo, req.GetStaticLabels()[repositoryLabel], "expected repo to match")
			assert.Equal(t, info.Env, req.GetStaticLabels()[environmentLabel], "expected env to match")
			assert.Equal(t, strconv.Itoa(int(info.WorkflowRunID)), req.GetStaticLabels()[workflowRunLabel], "expected workflow run ID to match")
		}

		t.Run("Sanity Check", func(t *testing.T) {
			storeWorkflowRunInfoTestFunc(t, GitHubWorkflowInfo{
				Org:           "test-org",
				Repo:          "test-repo",
				Env:           "test-env",
				WorkflowRunID: 12345,
			}, false)
		})

		t.Run("Missing Fields", func(t *testing.T) {
			tests := genMissingWorkflowInfo()
			require.Len(t, tests, 15, "expected 15 test cases for missing fields")

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					storeWorkflowRunInfoTestFunc(t, tt.info, true)
				})
			}
		})
	})

	t.Run("GetWorkflowRunInfo", func(t *testing.T) {
		t.Run("Sanity Check", func(t *testing.T) {
			req, err := types.NewAccessRequest(uuid.NewString(), "test-user", "test-role")
			require.NoError(t, err, "failed to create access request")
			req.SetStaticLabels(map[string]string{
				organizationLabel: "test-org",
				repositoryLabel:   "test-repo",
				environmentLabel:  "test-env",
				workflowRunLabel:  strconv.Itoa(12345),
			})
		})
	})
}

type missingLabelTestCases struct {
	name string
	info GitHubWorkflowInfo
}

// genMissingWorkflowInfo generates test cases for all combinations of missing labels in GitHubWorkflowInfo.
// This is a quick sanity check to ensure that we're not storing missing data in an external store which would require manual cleanup.
// It generates 15 test cases, each with a different combination of missing labels.
func genMissingWorkflowInfo() []missingLabelTestCases {
	list := []missingLabelTestCases{}

	// Use bitmask to generate all combinations of missing labels.
	// There are 4 labels, so we can represent all combinations with a 4-bit number (0-15).
	const (
		zeroOrg = 1 << iota
		zeroRepo
		zeroEnv
		zeroRunID
	)

	for i := int64(15); i > 0; i-- {
		info := GitHubWorkflowInfo{
			Org:           "test-org",
			Repo:          "test-repo",
			Env:           "test-env",
			WorkflowRunID: 12345,
		}
		missing := []string{}
		if i&zeroOrg == zeroOrg {
			info.Org = ""
			missing = append(missing, organizationLabel)
		}
		if i&zeroRepo == zeroRepo {
			info.Repo = ""
			missing = append(missing, repositoryLabel)
		}
		if i&zeroEnv == zeroEnv {
			info.Env = ""
			missing = append(missing, environmentLabel)
		}
		if i&zeroRunID == zeroRunID {
			info.WorkflowRunID = 0
			missing = append(missing, workflowRunLabel)
		}

		list = append(list, missingLabelTestCases{
			name: "missing: " + strings.Join(missing, ", "),
			info: info,
		})
	}

	return list
}

// FuzzStoreWorkflowInfo tests the StoreWorkflowInfo method of the GitHub store.
// This function is in a publicly accessible path and input can be manipulated by a malicious user.
// There is some validation in the StoreWorkflowInfo method, but we want to ensure that it doesn't panic or crash the service.
func FuzzStoreWorkflowInfo(f *testing.F) {
	f.Add("test-org", "test-repo", "test-env", int64(12345))
	f.Fuzz(func(t *testing.T, org, repo, env string, runID int64) {
		store, err := NewRepository()
		require.NoError(t, err, "failed to create GitHub store")

		req, err := types.NewAccessRequest(uuid.NewString(), "test-user", "test-role")
		require.NoError(t, err, "failed to create access request")

		info := GitHubWorkflowInfo{
			Org:           org,
			Repo:          repo,
			Env:           env,
			WorkflowRunID: runID,
		}

		err = store.GitHub.StoreWorkflowInfo(context.Background(), req, info)
		if err == nil {
			return
		}

		var missingLabelErr *MissingLabelError
		_, err = store.GitHub.GetWorkflowInfo(context.Background(), req)
		assert.ErrorAs(t, err, &missingLabelErr)
	})
}
