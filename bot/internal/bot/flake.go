package bot

import (
	"context"
	"os"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

const skipPrefix = "/excludeflake"

// ExcludeFlakes gets the list of test names that can be
// excluded from flaky test detection for a particular PR.
// Admin reviewers can exclude tests by commenting on a
// PR with "/excludeflake Test1 Test2".
//
// The result is written to a GitHub Action output parameter
// named FLAKE_SKIP.
func (b *Bot) ExcludeFlakes(ctx context.Context) error {
	skip, err := b.testsToSkip(ctx)
	if err != nil {
		return trace.Wrap(err)
	}

	outfile := os.Getenv(github.OutputEnv)
	err = os.WriteFile(outfile, []byte("FLAKE_SKIP="+strings.Join(skip, " ")), 0644)
	return trace.Wrap(err)
}

func (b *Bot) testsToSkip(ctx context.Context) ([]string, error) {
	comments, err := b.c.GitHub.ListComments(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var testsToSkip []string

	admins := b.c.Review.GetAdminCheckers(b.c.Environment.Author)
	for _, c := range comments {
		if !contains(admins, c.Author) {
			continue
		}
		if strings.HasPrefix(c.Body, skipPrefix) {
			for _, testName := range strings.Fields(c.Body)[1:] {
				testsToSkip = append(testsToSkip, testName)
			}
		}
	}

	return testsToSkip, nil
}
