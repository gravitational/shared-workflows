#!/usr/bin/env bash
#
# prs-between-tags.sh
#
# List all pull requests merged between two tags for a GitHub repo. Teleport
# development lives in the single gravitational/core monorepo, so one run covers
# both OSS and enterprise code:
#
#   scripts/prs-between-tags.sh v19.1.0 v19.1.1
#
# A repo can be passed as a third argument to override the default, e.g. to look
# at a range that predates the monorepo migration and still lives in the old
# gravitational/teleport or gravitational/teleport.e repos:
#
#   scripts/prs-between-tags.sh v18.9.1 v18.9.2 gravitational/teleport
#
# Each commit in the tag range is mapped to its PR via GitHub's commit->PR graph
# (the /commits/{sha}/pulls endpoint), not by scraping "(#1234)" out of the
# commit subject. That endpoint returns the merged PR that *introduced* the
# commit, so we get the actual backport PR on the release branch (correct author,
# correct number) rather than the original PR referenced in the subject, and we
# never trip over issue/backport refs that aren't real PRs.
#
# Output is one header line per PR followed by its changed files, indented:
#
#   #1234  Some title  (@author)  https://github.com/.../pull/1234
#       lib/foo/bar.go
#       lib/foo/bar_test.go
#
# Also scans the same commit range for a different signal, printed to
# stderr: any single commit whose message bundles an unusual number of
# "(#NNNN)" PR references. That's the fingerprint of a squash/RC-staging
# commit that folds several PRs' changes into the release line under one new
# SHA - which is exactly what can make this script list some of those same
# PRs again later as "new", once their *original* per-PR commits get merged
# into the release branch separately as a content no-op. See this skill's
# "Known failure mode" section, and run scripts/verify-not-shipped.py on the
# output before trusting it - don't rely on this warning alone.
#
# Requires an authenticated gh (`gh auth login`) with access to the private
# gravitational/core repo.
#
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "Usage: $(basename "$0") <old-tag> <new-tag> [owner/repo]" >&2
  exit 1
fi

OLD_TAG="$1"
NEW_TAG="$2"
REPO="${3:-gravitational/core}"

# Squash-commit warning (stderr only, best-effort - a failure here must never
# break the real output on stdout). A commit message bundling many "(#NNNN)"
# refs in one go means several PRs' changes landed in one shot, bypassing the
# usual one-commit-per-PR history; if those PRs' *original* commits later get
# merged into the release branch separately, this script will list them
# again here as if they were new. See SKILL.md's "Known failure mode".
{
  gh api --paginate "repos/${REPO}/compare/${OLD_TAG}...${NEW_TAG}" \
      --jq '.commits[] | .sha + "\t" + (.commit.message | gsub("\r?\n"; " "))' 2>/dev/null \
    | while IFS=$'\t' read -r sha message; do
        refs=$(grep -o '(#[0-9]\+)' <<< "$message" | wc -l | tr -d ' ')
        if [[ "$refs" -gt 3 ]]; then
          echo "warning: commit ${sha:0:12} bundles $refs PR refs in one message - possible squash/RC-staging commit (see SKILL.md 'Known failure mode'); run scripts/verify-not-shipped.py before trusting any of those PRs if they also appear below: ${message:0:160}"
        fi
      done
} >&2 || true

# For each commit in the range, ask GitHub which PR introduced it. The endpoint
# also returns open PRs that merely contain the commit (it isn't on the default
# branch), so keep only the merged one. Collect unique PR numbers.
gh api --paginate "repos/${REPO}/compare/${OLD_TAG}...${NEW_TAG}" \
    --jq '.commits[].sha' \
  | while read -r sha; do
      gh api "repos/${REPO}/commits/${sha}/pulls" \
        --jq 'map(select(.merged_at != null)) | first | .number // empty' 2>/dev/null
    done \
  | sort -un \
  | while read -r num; do
      # One header line per PR, followed by its changed files indented below.
      # The file list is what lets a no-risk PR (docs/tests/deps) be told apart
      # from one that touches production code.
      gh pr view "$num" --repo "$REPO" \
        --json number,title,author,url,files \
        --template '#{{.number}}  {{.title}}  (@{{.author.login}})  {{.url}}{{"\n"}}{{range .files}}    {{.path}}{{"\n"}}{{end}}' \
        2>/dev/null || echo "#${num}  (could not fetch from ${REPO})"
    done
