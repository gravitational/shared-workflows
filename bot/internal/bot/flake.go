package bot

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

const skipFlakePrefix = "/excludeflake"

// ExcludeFlakes gets the list of test names that can be
// excluded from flaky test detection for a particular PR.
// Admin reviewers can exclude tests by commenting on a
// PR with "/excludeflake Test1 Test2".
//
// The result is written to a GitHub Action output parameter
// named FLAKE_SKIP.
func (b *Bot) ExcludeFlakes(ctx context.Context) error {
	skip, err := b.skipItems(ctx, skipFlakePrefix)
	if err != nil {
		return trace.Wrap(err)
	}

	log.Printf("tests to skip: %v", strings.Join(skip, " "))

	output := "FLAKE_SKIP=" + strings.Join(skip, " ")
	outfile := os.Getenv(github.OutputEnv)
	err = os.WriteFile(outfile, []byte(output), 0644)

	log.Printf("wrote %q to %v", output, outfile)

	return trace.Wrap(err)
}
