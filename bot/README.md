# Teleport GitHub Review Bot

This is a Go GitHub PR automation bot used to perform various convenient tasks on Pull Requests.

## Usage

This bot is meant to be used in a \_.github/workflows/\<action>.yaml like so:

```yaml
jobs:
  check-reviews:
    name: Checking reviewers
    if: ${{ !github.event.pull_request.draft }}
    runs-on: ubuntu-latest
    steps:
      # Checkout master branch of Teleport repository. This is to prevent an
      # attacker from submitting their own review assignment logic.
      - name: Checkout master branch
        uses: actions/checkout@v2
        with:
          repository: gravitational/shared-workflows
          path: .github/workflows/robot
          ref: main
      - name: Installing the latest version of Go.
        uses: actions/setup-go@v2
      - name: Checking reviewers
        run: cd .github/workflows/robot && go run main.go -workflow=check -token="${{
          secrets.GITHUB_TOKEN }}"
```

## Workflows

This bot is capable of performing different actions, called workflows and selected with the `-workflow` argument.

### assign

Will assign reviewers for this PR.

Assign works by parsing the PR, discovering the changes, and returning a set of reviewers determined by: content of the
PR, if the author is internal or external, and team they are on.

### check

Checks if required reviewers have approved the PR.

Team specific reviews require an approval from both sets of reviews. External reviews require approval from admins.

### dismiss

Dismisses all stale workflow runs within a repository. This is done to dismiss stale workflow runs for external
contributors whose workflows run without permissions to dismiss stale workflows inline.

This is needed because GitHub appends each "Check" workflow run to the status of a PR instead of replacing the "Check"
status of the previous run.

Only dismiss stale runs from forks (external) as the workflow that triggers this method is intended for. Dismissing runs
for internal contributors (non-fork) here could result in a race condition as runs are deleted upon trigger separately
during the `Check` workflow.

### label

Adds labels to PRs.

If the PR is for a backport, adds the `backport` label

Otherwise, it uses a list of path prefixes and matching labels to pick the appropriate labels.

See [internal/bot/label.go#L99](internal/bot/label.go#L99) for the complete list

### backport

Will create backport Pull Requests (if requested) when a Pull Request is merged.

The branches targeted by the backports are controlled by the `backport/*` labels
