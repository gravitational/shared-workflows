package bot

import (
	"context"
	"log"
	"regexp"
	"slices"
	"strings"

	"github.com/gravitational/trace"
)

var (
	testPlanHeadingRegex  = regexp.MustCompile(`(?mi)^## Manual Test Plan\s*$`)
	nextHeadingRegex      = regexp.MustCompile(`(?m)^#{1,2} `)
	testEnvHeadingRegex   = regexp.MustCompile(`(?mi)^### Test Environment\s*$`)
	testCasesHeadingRegex = regexp.MustCompile(`(?mi)^### Test Cases\s*$`)
	subHeadingRegex       = regexp.MustCompile(`(?m)^### `)
	checkboxRegex         = regexp.MustCompile(`- \[([ xX])\]`)
)

// ValidateManualTestPlan checks that the PR description contains a manual test
// plan which details where the tests were run and which test cases were exercised.
// For example, the following used a cloud staging tenant named rjones to validate
// that foo, bar, and baz completed successfully.
//
// ## Manual Test Plan
//
// ### Test Environment
//
// rjones.cloud.gravitational.io
//
// ### Test Cases
// - [x] foo
// - [x] bar
// - [x] baz
func (b *Bot) ValidateManualTestPlan(ctx context.Context) error {
	pull, err := b.c.GitHub.GetPullRequest(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number,
	)
	if err != nil {
		return trace.Wrap(err, "failed to retrieve pull request for https://github.com/%s/%s/pull/%d", b.c.Environment.Organization, b.c.Environment.Repository, b.c.Environment.Number)
	}

	const noTestPlanLabel string = "no-test-plan"
	if slices.Contains(pull.UnsafeLabels, noTestPlanLabel) {
		log.Printf("PR contains %q label, skipping test plan check", noTestPlanLabel)
		return nil
	}

	if err := validateTestPlanContents(pull.UnsafeBody); err != nil {
		return trace.Wrap(err)
	}

	return nil
}

// validateTestPlanContents validates that the PR body contains a well-formed manual test plan section.
func validateTestPlanContents(body string) error {
	// Find the ## Manual Test Plan heading.
	loc := testPlanHeadingRegex.FindStringIndex(body)
	if loc == nil {
		return trace.BadParameter(`The PR description must contain a "Manual Test Plan" section, please add one, or a "no-test-plan" label if a test plan does not apply to this change`)
	}

	// Extract section content from after the heading to the next same-level or higher heading.
	sectionStart := loc[1]
	section := body[sectionStart:]
	if nextLoc := nextHeadingRegex.FindStringIndex(section); nextLoc != nil {
		section = section[:nextLoc[0]]
	}

	// Find the ### Test Environment sub-heading and verify it has content.
	envLoc := testEnvHeadingRegex.FindStringIndex(section)
	if envLoc == nil {
		return trace.BadParameter(`The "Manual Test Plan" section must contain a "Test Environment" sub-heading detailing where the tests were run`)
	}
	envContent := section[envLoc[1]:]
	if nextSub := subHeadingRegex.FindStringIndex(envContent); nextSub != nil {
		envContent = envContent[:nextSub[0]]
	}
	if strings.TrimSpace(envContent) == "" {
		return trace.BadParameter(`The "Test Environment" section must not be empty`)
	}

	// Check for a ### Test Cases sub-heading.
	if !testCasesHeadingRegex.MatchString(section) {
		return trace.BadParameter(`The "Manual Test Plan" section must contain a "Test Cases" sub-heading detailing the test plan`)
	}

	// Find all checkboxes.
	matches := checkboxRegex.FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		return trace.BadParameter(`The "Manual Test Plan" section must contain at least one test case`)
	}

	// Verify all checkboxes are checked.
	for _, match := range matches {
		if strings.TrimSpace(match[1]) == "" {
			return trace.BadParameter("All test cases have not yet been completed")
		}
	}

	return nil
}
