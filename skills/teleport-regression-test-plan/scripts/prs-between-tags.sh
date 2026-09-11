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
