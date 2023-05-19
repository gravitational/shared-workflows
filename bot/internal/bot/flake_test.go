package bot

import (
	"context"
	"testing"
	"time"

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
				comment("admin1", "/excludeflake TestFoo"),
			},
			skip: []string{"TestFoo"},
		},
		{
			desc: "missing test",
			comments: []github.Comment{
				comment("admin1", "/excludeflake  "),
			},
			skip: nil,
		},
		{
			desc: "missing prefix",
			comments: []github.Comment{
				comment("admin1", "TestFoo TestBar"),
			},
			skip: nil,
		},
		{
			desc: "missing test",
			comments: []github.Comment{
				comment("admin1", "abc"),
				comment("admin2", "def"),
				comment("bob", "ghi"),
				comment("alice", "jkl"),
			},
			skip: nil,
		},
		{
			desc: "multiple",
			comments: []github.Comment{
				comment("admin1", "/excludeflake TestFoo TestBar"),
			},
			skip: []string{"TestFoo", "TestBar"},
		},
		{
			desc: "complex",
			comments: []github.Comment{
				comment("admin1", "/excludeflake TestFoo TestBar"),
				comment("nonadmin", "/excludeflake TestBaz"),
				comment("admin2", "/excludeflake TestQuux"),
			},
			skip: []string{"TestFoo", "TestBar", "TestQuux"},
		},
		{
			desc: "comment updated",
			comments: []github.Comment{
				comment("admin1", "/excludeflake TestFoo"),
				{
					Author:    "admin2",
					Body:      "/excludeflake TestBar",
					CreatedAt: time.Now().Add(-10 * time.Minute),
					UpdatedAt: time.Now(),
				},
			},
			skip: []string{"TestFoo"},
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

func comment(author, body string) github.Comment {
	now := time.Now()
	return github.Comment{
		Author:    author,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
