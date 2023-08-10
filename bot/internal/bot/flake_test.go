package bot

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
	"github.com/stretchr/testify/require"
)

func TestSkipFlakes(t *testing.T) {
	r, err := review.New(&review.Config{
		Admins:            []string{"admin1", "admin2"},
		CodeReviewers:     make(map[string]review.Reviewer),
		CodeReviewersOmit: make(map[string]bool),
		DocsReviewers:     make(map[string]review.Reviewer),
		DocsReviewersOmit: make(map[string]bool),
	})
	require.NoError(t, err)

	b := &Bot{
		c: &Config{
			Environment: &env.Environment{},
			GitHub: &fakeGithub{comments: map[int][]github.Comment{
				0: {
					comment("admin1", "/excludeflake TestFoo TestBar"),
					comment("nonadmin", "/excludeflake TestBaz"),
					comment("admin2", "/excludeflake TestQuux"),
				}},
			},
			Review: r,
		},
	}

	// Create a temp file for the skipped tests to be written to
	f, err := os.CreateTemp(t.TempDir(), "flake_skips")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	t.Setenv(github.OutputEnv, f.Name())

	// Validate that only the entries excluded by admins exist in the output
	require.NoError(t, b.ExcludeFlakes(context.Background()))
	actual, err := io.ReadAll(f)
	require.NoError(t, err)
	require.Equal(t, "FLAKE_SKIP=TestFoo TestBar TestQuux", string(actual))
}
