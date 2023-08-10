/*
Copyright 2021 Gravitational, Inc.

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
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

const newDocsReminderList = `Thanks for writing a new docs page!

Please don't forget to:

- [ ] Read the [Docs Contribution Guide](https://goteleport.com/docs/contributing/documentation/) if you haven't already.
- [ ] Add backport labels (&#96;backport/branch/v<number>&#96;) for the current and two major Teleport versions (if applicable for your feature) 
- [ ] Make sure you list any new docs pages in &#96;docs/config.json&#96;.
- [ ] Document new functionality in a how-to guide, reference docs, and architectural docs (or create new issues to do this later).
`

const (
	docsPrefix     = "docs/pages"
	docsSuffix     = ".mdx"
	includeSegment = "/includes/"
)

func (b *Bot) remind(ctx context.Context, number int, files []github.PullRequestFile) error {
	var newDoc bool
	for _, file := range files {
		if !strings.HasPrefix(file.Name, docsPrefix) {
			continue
		}
		if !strings.HasSuffix(file.Name, docsSuffix) {
			continue
		}
		if strings.Contains(file.Name, includeSegment) {
			continue
		}
		if file.Status == github.StatusAdded {
			newDoc = true
		}
	}

	if !newDoc {
		return nil
	}

	c, err := b.c.GitHub.ListComments(ctx, b.c.Environment.Organization, b.c.Environment.Repository, number)
	if err != nil {
		return trace.Wrap(err)
	}

	// We have already sent the new docs reminder
	for _, m := range c {
		if m.Body == newDocsReminderList {
			return nil
		}
	}

	b.c.GitHub.CreateComment(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		number,
		newDocsReminderList,
	)

	return nil
}

// Remind adds reminders to the comments of outstanding pull requests.
func (b *Bot) Remind(ctx context.Context) error {
	files, err := b.c.GitHub.ListFiles(
		ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err)
	}

	return b.remind(
		ctx,
		b.c.Environment.Number,
		files,
	)
}
