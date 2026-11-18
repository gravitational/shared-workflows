package bot

import (
	"context"
	"slices"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/trace"
)

const (
	rfdCoreReviewLabel  = "review/core"
	rfdCloudReviewLabel = "review/cloud"
)

// selectRFDReviewTeam reads current PR labels so label changes take effect on reruns.
func (b *Bot) selectRFDReviewTeam(ctx context.Context) error {
	if b.c.Environment.Repository != env.RFDRepo {
		return nil
	}
	pull, err := b.c.GitHub.GetPullRequest(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	core := slices.Contains(pull.UnsafeLabels, rfdCoreReviewLabel)
	cloud := slices.Contains(pull.UnsafeLabels, rfdCloudReviewLabel)
	if core && cloud {
		return trace.BadParameter("RFD PR cannot have both %q and %q labels", rfdCoreReviewLabel, rfdCloudReviewLabel)
	}

	b.c.Environment.RFDReviewTeam = ""
	switch {
	case core:
		b.c.Environment.RFDReviewTeam = env.CoreTeam
	case cloud:
		b.c.Environment.RFDReviewTeam = env.CloudTeam
	}
	return trace.Wrap(b.c.Review.ValidateRFDReviewTeam(b.c.Environment))
}
