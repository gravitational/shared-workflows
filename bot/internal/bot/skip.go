package bot

import (
	"context"
	"log"
	"strings"

	"github.com/gravitational/trace"
	"golang.org/x/exp/slices"
)

// skipItems finds any comments from an admin with the skipPrefix
// and returns the list of items that may be skipped.
func (b *Bot) skipItems(ctx context.Context, skipPrefix string) ([]string, error) {
	// If the event was not from a PullRequest then there
	// will be no comments.
	if b.c.Environment.Number == 0 {
		return nil, nil
	}

	comments, err := b.c.GitHub.ListComments(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var itemsToSkip []string

	admins := b.c.Review.GetAdminCheckers(b.c.Environment.Author)
	for _, c := range comments {
		if !slices.Contains(admins, c.Author) {
			log.Printf("ignoring comment from non-admin %v", c.Author)
			continue
		}
		// non-admins can edit comments from admins, so we only
		// consider comments that have not been updated
		if !c.CreatedAt.IsZero() && c.CreatedAt != c.UpdatedAt {
			log.Printf("ignoring edited comment from %v", c.Author)
			continue
		}
		if strings.HasPrefix(c.Body, skipPrefix) {
			for _, testName := range strings.Fields(c.Body)[1:] {
				itemsToSkip = append(itemsToSkip, testName)
			}
		}
	}

	return itemsToSkip, nil
}
