#!/bin/bash
set -euo pipefail

# This eventually needs to move to a separate project along side the Renovate workflow.
# This script update changelogs ever time there is a dependency update, which Renovate
# then includes in the update PR.

if [ "$#" -lt 2 ]; then
    echo "Usage: $0 <updated file path> <changelog entry>" >&2
    exit 1
fi

UPDATED_FILE_PATH="$1"
CHANGELOG_ENTRY="${*:2}"

search_upwards_for_changelog() {
    # Based on https://superuser.com/a/1752082 with some changes
    SEARCH_DIR=$(realpath "$(dirname "$UPDATED_FILE_PATH")")
    while RESULT=$(find "$SEARCH_DIR"/ -maxdepth 1 -type f -name "CHANGELOG.md")
    # If result not found and not at the repo root (which contains the .git directory)
    [ -z "$RESULT" ] && [ -z "$(find "$SEARCH_DIR"/ -maxdepth 1 -type d -name '.git')" ]
    do SEARCH_DIR=$(dirname "$SEARCH_DIR"); done

    realpath "$RESULT"
}

CHANGELOG_PATH=$(search_upwards_for_changelog)

if [ -z "$CHANGELOG_PATH" ]; then
    echo "No changelog found in a directory above $UPDATED_FILE_PATH, skipping update" >&2
    exit
fi

if grep --quiet "$CHANGELOG_ENTRY" "$CHANGELOG_PATH"; then
    echo "Changelog entry '$CHANGELOG_ENTRY' already found in changelog at $CHANGELOG_PATH, skipping update" >&2
    exit
fi

if ! command -v "chan"; then
    echo "chan NPM module not found, installing" >&2
    # TODO manage this with Renovate once this script and the action are pulled into a separate project
    npm install --global '@geut/chan@3.2.9'
fi

CHANGELOG_DIRECTORY=$(dirname "$CHANGELOG_PATH")

echo "Updating changelog $CHANGELOG_PATH with entry '$CHANGELOG_ENTRY'"
cd "$CHANGELOG_DIRECTORY" && chan changed -g "Dependency Updates" "$CHANGELOG_ENTRY"
