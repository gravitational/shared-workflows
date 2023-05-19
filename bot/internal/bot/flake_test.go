package bot

import (
	"context"
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

	for _, test := range []struct {
		desc     string
		comments []github.Comment
		skip     []string
	}{
		{
			desc:     "empty",
			comments: nil,
			skip:     nil,
		},
		{
			desc: "simple",
			comments: []github.Comment{
				{Author: "admin1", Body: "/excludeflake TestFoo"},
			},
			skip: []string{"TestFoo"},
		},
		{
			desc: "missing test",
			comments: []github.Comment{
				{Author: "admin1", Body: "/excludeflake  "},
			},
			skip: nil,
		},
		{
			desc: "missing prefix",
			comments: []github.Comment{
				{Author: "admin1", Body: "TestFoo TestBar"},
			},
			skip: nil,
		},
		{
			desc: "missing test",
			comments: []github.Comment{
				{Author: "admin1", Body: "abc"},
				{Author: "admin2", Body: "def"},
				{Author: "bob", Body: "ghi"},
				{Author: "alice", Body: "jkl"},
			},
			skip: nil,
		},
		{
			desc: "multiple",
			comments: []github.Comment{
				{Author: "admin1", Body: "/excludeflake TestFoo TestBar"},
			},
			skip: []string{"TestFoo", "TestBar"},
		},
		{
			desc: "complex",
			comments: []github.Comment{
				{Author: "admin1", Body: "/excludeflake TestFoo TestBar"},
				{Author: "nonadmin", Body: "/excludeflake TestBaz"},
				{Author: "admin2", Body: "/excludeflake TestQuux"},
			},
			skip: []string{"TestFoo", "TestBar", "TestQuux"},
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			b := &Bot{
				c: &Config{
					Environment: &env.Environment{},
					GitHub:      &fakeGithub{comments: test.comments},
					Review:      r,
				},
			}
			skip, err := b.testsToSkip(context.Background())
			require.NoError(t, err)
			require.ElementsMatch(t, skip, test.skip)
		})
	}
}
