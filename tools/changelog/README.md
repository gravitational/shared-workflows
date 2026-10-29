# changelog

This script generates a changelog for a release.

## Usage

```shell
usage: changelog [<flags>]

Flags:
  --[no-]help               Show context-sensitive help (also try --help-long and --help-man).
  --base-branch=BASEBRANCH  The base release branch to generate the changelog for, e.g. branch/v18 or branch/v18.11.8 ($BASE_BRANCH)
  --base-tag=BASETAG        The tag/version to generate the changelog from. It will be of the form vXX.Y.Z, e.g. v15.1.1. A release branch also works ($BASE_TAG)
  --pr-links                Include markdown links to GitHub pull requests. ($PR_LINKS)
  --pr-number-cutoff=DATE   The date (YYYY-MM-DD) from which a commit's "(#N)" suffix is taken to be a PR in this repo ($PR_NUMBER_CUTOFF)
  --debug                   Print commit ranges, commits, PR numbers, and suppressed entries used to generate the changelog. ($DEBUG)
```

It can optionally take two input variables: BASE_BRANCH: The base release
branch to generate the changelog for. It will be of the form "branch/v*".
BASE_TAG: The tag/version to generate the changelog from. It will be of the
form "vXX.Y.Z", e.g. "v15.1.1"

**Generate from the release branch, not the base branch's tip.** Pass the ref
the release is actually cut from, e.g.
`--base-branch=branch/v18.11.8 --base-tag=v18.11.7`. Running against
`branch/v18`'s tip instead announces whatever has landed there since the
release branch was cut, which is work the release does not contain.


If neither are provided, the values will be automatically determined if
possible:
* The current branch will be used as the base branch if it matches the
  pattern branch/v*
* If the current branch is forked from a base branch, the base branch will be
  used. e.g. if you create release/15.1.1 from branch/v15, the branch/v15
  will be the base branch.
* The base tag will be determined by running "make print-version" from the
  root of the repo.


Enterprise PR changelogs will be listed after the OSS changelogs. PR links are
omitted by default because private repo links are not useful to external users.
Pass `--pr-links` when you want changelog entries to include markdown links to
their source PRs.

If you reword changelogs, it is best to go to the source PR and change it
there and then regenerate the changelog.


## Chained release branches

Release branches do not always descend from the previous release's tag. A patch
release may be chained off the previous patch branch and take base branch work
through squash merges and cherry-picks, so that private security fixes can ship
without everything else that has landed on the base branch.

A plain `base-tag..base-branch` commit range over-reports in that case. Squash
merges discard the commits they collapse, so git sees no ancestry between the
release branch and work that shipped several releases ago, and reports all of
it as new. For `branch/v18.11.7..branch/v18.11.8` that was 252 pull requests
where 215 were new.

So entries already released from the base tag are removed before the changelog
is rendered, matching on:

* the pull request number, for commits reachable from the base tag
* the commit subject, for work that arrived through a squash merge or that was
  authored in a private fork and so carries a foreign pull request number
* the subjects of the commits behind each pull request merged into the base
  tag's chain, fetched from GitHub - the only way back to the commits inside a
  squash merge whose message was trimmed to the pull request title

Only the base tag's commits since it diverged from the base branch are
considered. When the base tag is an ancestor of the base branch - the ordinary
case - nothing is removed and the output is the plain commit range, unchanged.

Run with `--debug` to see which entries were removed and why. Nothing is
reported otherwise.

### PR number collisions

A commit subject's `(#N)` suffix does not identify a pull request on its own.
This repository was renamed and restarted its numbering, and private security
forks number their own pull requests, so the same number appears in history for
several different pull requests. `--pr-number-cutoff` sets the date from which
a number is taken to belong to this repository; numbers on commits before it
are ignored, and numbers inside squash merges are never trusted. Two commits
after the cutoff that share a number and are both reachable from the base tag
would still be conflated.


Caveats:
* If you update the "e" ref in your release PR, and you also run `make
  changelog` from the release PR branch, if you have already updated the
  version in the makefile, you will need to run `make changelog
  BASE_TAG=X.Y.Z` as this script will determine the base tag to be the
  current release version not the last released version.


One preferred way of using this script is to run `make changelog` from the
base branch and save it: `make changelog > /tmp/changelog`. If any PRs are
merged to the base branch after you have created your release PR but before
you have merged it, you can see any new entries with:

```shell
git checkout branch/vNN
diff -u /tmp/changelog $(make changelog)
```
If there are changes, you can update your changelog and rebase your branch:
```shell
git pull on branch/vNN
make changelog > /tmp/changelog
git checkout release/XX.Y.Z
git rebase branch/vNN
<include /tmp/changelog in CHANGELOG.md>
git add CHANGELOG.md && git commit --amend --no-edit && git push -f
```

Ensure you update the PR body with the new changelog, doable on the command
line with `gh`:
```
gh pr edit --body-file /tmp/changelog
```
