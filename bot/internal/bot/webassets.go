/*
Copyright 2025 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bot

import (
	"context"
	"slices"
	"strings"

	"github.com/gravitational/trace"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/shared-workflows/bot/internal/webassets"
)

func (b *Bot) CompareWebAssetsStats(ctx context.Context, statsPaths string) error {
	beforePath, afterPath, err := parseStatsPaths(statsPaths)
	if err != nil {
		return trace.Wrap(err)
	}

	before, err := webassets.LoadStats(beforePath)
	if err != nil {
		return trace.Wrap(err)
	}

	after, err := webassets.LoadStats(afterPath)
	if err != nil {
		return trace.Wrap(err)
	}

	comparison, err := webassets.Compare(before, after)
	if err != nil {
		return trace.Wrap(err)
	}

	return b.reconcileComment(ctx, comparison)
}

func (b *Bot) reconcileComment(ctx context.Context, body string) error {
	comments, err := b.c.GitHub.ListComments(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err)
	}

	existingCommentIndex := slices.IndexFunc(comments, func(c github.Comment) bool {
		return webassets.IsBotComment(c.Body)
	})

	if existingCommentIndex >= 0 {
		comment := comments[existingCommentIndex]

		return trace.Wrap(b.c.GitHub.EditComment(ctx,
			b.c.Environment.Organization,
			b.c.Environment.Repository,
			comment.ID,
			body,
		))
	}

	return trace.Wrap(b.c.GitHub.CreateComment(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
		body,
	))
}

func parseStatsPaths(statsPaths string) (beforeStatsPath, afterStatsPath string, err error) {
	split := strings.Split(statsPaths, ",")
	if len(split) != 2 {
		return "", "", trace.BadParameter("expected two paths separated by a comma, got %q", statsPaths)
	}

	beforeStatsPath = strings.TrimSpace(split[0])
	afterStatsPath = strings.TrimSpace(split[1])

	if beforeStatsPath == "" || afterStatsPath == "" {
		return "", "", trace.BadParameter("both paths must be non-empty")
	}

	return beforeStatsPath, afterStatsPath, nil
}
