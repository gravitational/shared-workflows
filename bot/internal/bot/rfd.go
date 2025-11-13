package bot

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

// ValidateNewRFD ensures that PRs which add **new** RFDs follow
// the process laid out in [RFD 0](https://github.com/gravitational/teleport/blob/61ed36979ecb98310c853a6535108f57464cbef2/rfd/0000-rfds.md).
// Namely, this ensures that
// - All branches are in the form rfd/$number-your-title
// - The RFD itself exists at /rfd/$number-your-title.md
// - All RFD numbers are properly zero padded to avoid collisions (rfd/123-foo vs. rfd/0123-bar)
func (b *Bot) ValidateNewRFD(ctx context.Context) error {
	files, err := b.c.GitHub.ListFiles(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	branchRegexp := regexp.MustCompile(`^rfd\/(\d+)-`)
	matches := branchRegexp.FindStringSubmatch(b.c.Environment.UnsafeHead)
	isRFDBranch := len(matches) == 2

	var hasValidRFD bool
	for _, file := range files {
		if file.Status != github.StatusAdded {
			continue
		}

		if strings.HasPrefix(file.Name, "rfd/assets/") || !strings.HasPrefix(file.Name, "rfd/") {
			continue
		}

		if !isRFDBranch {
			return trace.BadParameter("RFD branches must follow the pattern rfd/$number-your-title")
		}

		if file.Name != b.c.Environment.UnsafeHead+".md" {
			return trace.BadParameter("Found RFD named %q, expected RFD to be named %q", file.Name, b.c.Environment.UnsafeHead+".md")
		}

		rfdNumberString := matches[1]
		rfdNumber, err := strconv.Atoi(rfdNumberString)
		if err != nil {
			return trace.BadParameter("RFD number %q is not a valid number", rfdNumberString)
		}

		expectedNumber := fmt.Sprintf("%04d", rfdNumber)
		if expectedNumber != rfdNumberString {
			return trace.BadParameter("Found branch named %q, expected branch to be named %q", b.c.Environment.UnsafeHead, "rfd/"+expectedNumber+b.c.Environment.UnsafeHead[4+len(rfdNumberString):])
		}

		hasValidRFD = true
	}

	if isRFDBranch && !hasValidRFD {
		return trace.BadParameter("RFD %q is missing", b.c.Environment.UnsafeHead+".md")
	}

	return nil
}
