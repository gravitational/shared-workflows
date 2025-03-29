package githubevents

import (
	"context"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubEvents(t *testing.T) {
	t.Run("Webhook", func(t *testing.T) {
		// Testing webhook by sending POST requests to a fake server
		webhook := NewSource(
			config.GitHubEvents{
				Address: "localhost:0",
			},
			&fakeProcessor{},
		)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		require.NoError(t, webhook.Setup(ctx))
		errc := make(chan error)
		go func() {
			errc <- webhook.Run(ctx)
		}()

		cl := &http.Client{
			Timeout: 10 * time.Second, // matches GitHub's timeout for webhooks
		}

		filepath.WalkDir("testdata", func(path string, d fs.DirEntry, err error) error {
			require.NoError(t, err)
			if d.IsDir() {
				return nil
			}

			payloadFile, err := os.Open(path)
			require.NoError(t, err)
			defer payloadFile.Close()

			testURL, err := url.JoinPath("http://", webhook.getAddr(), "/webhook")
			require.NoError(t, err)
			req, err := http.NewRequest("POST", testURL, payloadFile)
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "deployment_review")

			resp, err := cl.Do(req)
			assert.NoError(t, err)
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			return nil
		})

		cancel()
		require.NoError(t, <-errc)
	})
}

type fakeProcessor struct {
}

func (f *fakeProcessor) ProcessDeploymentReviewEvent(event DeploymentReviewEvent, valid bool) error {
	return nil
}
