---
name: teleport-regression-test-plan
description: Build a regression-testing checklist of PRs merged between two Teleport release tags. Fetches PRs from the gravitational/core monorepo between two tags, filters out PRs that pose no regression risk (docs-only, tests-only, dependency updates, and release PRs), and outputs a GitHub-flavored markdown artifact grouping the remaining PRs by author. Use when preparing for release regression testing or when asked "what changed between tag A and tag B".
---

# Teleport regression test plan

Builds a markdown checklist of pull requests that may carry regression risk
between two Teleport release tags, grouped by author, to scope manual regression
testing for a release.

Teleport development lives in the single `gravitational/core` monorepo. OSS and
enterprise code are both in it (enterprise under `e/`), so one pass over one
repo covers the whole release.

## Prerequisites

- `gh` CLI authenticated with access to the private `gravitational/core` repo
  (check with `gh auth status`).

## Step 1 — Fetch PRs

Run `scripts/prs-between-tags.sh` with the two tags. For each PR it prints a
header line `#<number>  <title>  (@<author>)  <url>` followed by the PR's
changed files, indented four spaces:

```
#1234  Some title  (@author)  https://github.com/gravitational/core/pull/1234
    lib/foo/bar.go
    lib/foo/bar_test.go
```

```bash
cd skills/teleport-regression-test-plan
./scripts/prs-between-tags.sh <OLD_TAG> <NEW_TAG> > /tmp/prs.txt
```

Each commit costs two API calls (resolve its PR, then fetch the PR), so a
release-sized range takes a minute or two. The script maps commits to PRs via
GitHub's commit→PR graph, so the number, title, and author are the real
release-branch (backport) PR — there is no double-listing of an original PR and
its backport, and no false hits on issue references. Keep the PR count (header
lines, `grep -c '^#'`) so you can report how many were filtered.

The monorepo migration landed in mid-August 2026. Commits older than that were
imported into `core`, and their introducing PRs live in the archived
`gravitational/teleport` and `gravitational/teleport.e` repos — a range of
pre-migration tags resolves to few or no PRs in `core`. For such a historical
range, pass the old repo as the script's third argument and run it once per
repo, combining the output:

```bash
./scripts/prs-between-tags.sh <OLD_TAG> <NEW_TAG> gravitational/teleport   > /tmp/oss-prs.txt
./scripts/prs-between-tags.sh <OLD_TAG> <NEW_TAG> gravitational/teleport.e > /tmp/e-prs.txt
```

## Step 2 — Filter out no-risk PRs

Drop PRs that cannot cause a regression, judging from the changed files printed
under each PR (fall back to the title only when a PR has no files listed).

Remove a PR when it falls **entirely** into one of the file-based buckets below.
A PR that mixes any production code change with docs/tests stays in — *except*
for the title- and author-based buckets (dependency updates, releases, docs
maintainers), which are dropped even when they also touch source files.

- **Dependency updates** — third-party version bumps. Drop on title even when
  the PR also edits source to adapt to the new version: `Bump <dep> from X to
  Y`, `Update <dep> to vX`, `Update to <dep> vX`. Also anything authored by
  `dependabot`/`renovate`, or PRs whose only files are dependency manifests
  (`go.mod`, `go.sum`, `package.json`, `pnpm-lock.yaml`, `Cargo.toml`,
  `Cargo.lock`, etc.).
- **Release PRs** — release-engineering changes that ship no product code:
  cutting a release or editing release metadata. Titles like `Release X.Y.Z`,
  `Update version and changelog to X`, `Remove prerelease note ...`; files are
  limited to version files, `CHANGELOG.md`, and the like. The release manager
  authors these — drop their version/changelog/release PRs.
- **Docs-only** — every changed file is under `docs/`. Titles often start with
  `docs:` or `[docs]`. Documentation maintainers author docs-only PRs whose
  titles don't always carry a `docs:` prefix (e.g. `Add a
  Configuring Teleport docs section`) — treat their docs work as docs and drop
  it even if a stray non-`docs/` file is included.
- **Tests-only** — every changed file is a test (`*_test.go`, `testdata/`,
  `*.test.ts`/`*.test.tsx`, `*_test.py`, `integration/`, `e2e/`, `e/e2e/`,
  `e/tests/`). Titles like `Fix flaky test ...` are common signals (verify the
  files).

Note that `e/` is enterprise product code, not vendored or generated content:
treat changes under it exactly like changes under `lib/` or `web/`.

When in doubt for a code PR, keep it — over-inclusion is the safe direction for
regression scoping. The author-based rules above are the deliberate exception:
release, dependency-bump, and docs-maintainer PRs are low risk by nature and
should be dropped even when uncertain.

## Step 3 — Render grouped markdown

Format the surviving PRs by following `template.md` in this skill directory.
Group by author, sort authors alphabetically, list each PR as a checkbox linking
to the PR, and fill in the summary counts (kept / total / filtered).

Output the rendered markdown directly as an artifact in your response. Emit it
inside a fenced ``` markdown code block so the user can read it rendered and
copy the raw source.

Follow `template.md` exactly for structure; the comment block in it lists the
formatting rules (link format, title cleanup, sort order) and must be omitted
from the final output.
