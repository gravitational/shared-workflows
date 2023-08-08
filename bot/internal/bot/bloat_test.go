package bot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/review"
	"github.com/stretchr/testify/require"
)

func createFileWithSize(t *testing.T, path string, sizeInMB int64) {
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(sizeInMB<<20))
}

func TestBloat(t *testing.T) {
	r, err := review.New(&review.Config{
		Admins:            []string{"admin1", "admin2"},
		CodeReviewers:     make(map[string]review.Reviewer),
		CodeReviewersOmit: make(map[string]bool),
		DocsReviewers:     make(map[string]review.Reviewer),
		DocsReviewersOmit: make(map[string]bool),
	})
	require.NoError(t, err)

	base := t.TempDir()
	current := t.TempDir()

	cases := []struct {
		name            string
		comments        []github.Comment
		createArtifacts func(t *testing.T, base, current string)
		errAssertion    require.ErrorAssertionFunc
		outAssertion    func(t *testing.T, out string)
	}{
		{
			name: "bloat skipped by admin",
			comments: []github.Comment{
				comment("admin1", "/excludebloat three"),
			},
			createArtifacts: func(t *testing.T, base, current string) {
				createFileWithSize(t, filepath.Join(base, "one"), 1)
				createFileWithSize(t, filepath.Join(current, "one"), 1)

				createFileWithSize(t, filepath.Join(base, "two"), 1)
				createFileWithSize(t, filepath.Join(current, "two"), 2)

				createFileWithSize(t, filepath.Join(base, "three"), 1)
				createFileWithSize(t, filepath.Join(current, "three"), 5)
			},
			errAssertion: require.NoError,
			outAssertion: func(t *testing.T, out string) {
				require.Equal(t, `        | Binary | Base Size | Current Size | Change                 |
        |--------|-----------|--------------|------------------------|
        | two    | 1MB       | 2MB          | 1MB ✅                  |
        | three  | 1MB       | 5MB          | 4MB ✅ skipped by admin |
        | one    | 1MB       | 1MB          | 0MB ✅                  |`, out)
			},
		},
		{
			name: "bloat detected",
			comments: []github.Comment{
				comment("nonadmin", "/excludebloat three"),
			},
			createArtifacts: func(t *testing.T, base, current string) {
				createFileWithSize(t, filepath.Join(base, "one"), 1)
				createFileWithSize(t, filepath.Join(current, "one"), 1)

				createFileWithSize(t, filepath.Join(base, "two"), 1)
				createFileWithSize(t, filepath.Join(current, "two"), 2)

				createFileWithSize(t, filepath.Join(base, "three"), 1)
				createFileWithSize(t, filepath.Join(current, "three"), 5)
			},
			errAssertion: require.Error,
			outAssertion: func(t *testing.T, out string) {
				require.Equal(t, `        | Binary | Base Size | Current Size | Change |
        |--------|-----------|--------------|--------|
        | one    | 1MB       | 1MB          | 0MB ✅  |
        | two    | 1MB       | 2MB          | 1MB ✅  |
        | three  | 1MB       | 5MB          | 4MB ❌  |`, out)
			},
		},
		{
			name: "artifact not found",
			createArtifacts: func(t *testing.T, base, current string) {
				createFileWithSize(t, filepath.Join(base, "one"), 1)
				createFileWithSize(t, filepath.Join(current, "one"), 1)

				createFileWithSize(t, filepath.Join(base, "two"), 1)
				createFileWithSize(t, filepath.Join(current, "two"), 2)
			},
			errAssertion: require.Error,
			outAssertion: func(t *testing.T, out string) {
				require.Empty(t, out)
			},
		},
		{
			name: "no bloat",
			createArtifacts: func(t *testing.T, base, current string) {
				createFileWithSize(t, filepath.Join(base, "one"), 1)
				createFileWithSize(t, filepath.Join(current, "one"), 1)

				createFileWithSize(t, filepath.Join(base, "two"), 1)
				createFileWithSize(t, filepath.Join(current, "two"), 1)

				createFileWithSize(t, filepath.Join(base, "three"), 1)
				createFileWithSize(t, filepath.Join(current, "three"), 1)
			},
			errAssertion: require.NoError,
			outAssertion: func(t *testing.T, out string) {
				require.Equal(t, `        | Binary | Base Size | Current Size | Change |
        |--------|-----------|--------------|--------|
        | one    | 1MB       | 1MB          | 0MB ✅  |
        | two    | 1MB       | 1MB          | 0MB ✅  |
        | three  | 1MB       | 1MB          | 0MB ✅  |`, out)
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{},
					GitHub:      &fakeGithub{comments: test.comments},
					Review:      r,
				},
			}

			test.createArtifacts(t, base, current)

			// Validate that only the entries excluded by admins exist in the output
			var out bytes.Buffer
			test.errAssertion(t, b.BloatCheck(context.Background(), base, current, []string{"one", "two", "three"}, &out))
		})
	}

}
