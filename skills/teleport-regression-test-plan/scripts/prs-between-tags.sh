#!/usr/bin/env bash
#
# prs-between-tags.sh
#
# List all pull requests merged between two tags for a GitHub repo. teleport and
# teleport.e share the same tags, so run it once per repo:
#
#   scripts/prs-between-tags.sh gravitational/teleport     v18.9.1 v18.9.2
#   scripts/prs-between-tags.sh gravitational/teleport.e   v18.9.1 v18.9.2
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
# Requires an authenticated gh (`gh auth login`).
#
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $(basename "$0") <owner/repo> <old-tag> <new-tag>" >&2
  exit 1
fi

REPO="$1"
OLD_TAG="$2"
NEW_TAG="$3"

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
      # The file list is what lets a no-risk PR (docs/tests/deps/e-bump) be told
      # apart from one that touches production code.
      gh pr view "$num" --repo "$REPO" \
        --json number,title,author,url,files \
        --template '#{{.number}}  {{.title}}  (@{{.author.login}})  {{.url}}{{"\n"}}{{range .files}}    {{.path}}{{"\n"}}{{end}}' \
        2>/dev/null || echo "#${num}  (could not fetch from ${REPO})"
    done
