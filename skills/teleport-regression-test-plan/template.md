# Regression testing: {OLD_TAG} → {NEW_TAG}

PRs that may carry regression risk, grouped by author. Docs-only, tests-only,
dependency updates, and `e` submodule bump PRs are excluded.

- **{KEPT}** PRs to review (out of {TOTAL} total across both repos; {FILTERED} filtered as no-risk)

## @{author1}

- [ ] {PR title} [#{number}]({url})
- [ ] {PR title} [#{number}]({url})

## @{author2}

- [ ] {PR title} [#{number}]({url})

<!--
Rules for filling this in:
- One `## @author` section per author, sorted alphabetically (case-insensitive).
- Within a section, list PRs as `- [ ]` checkboxes, OSS and teleport.e together,
  sorted by PR number.
- Link text is `#<number>`; the href is the PR url from the script output.
- Strip any trailing "(#NNNNN)" backport ref from the title — the link shows it.
- Omit this comment block from the final output.
-->
