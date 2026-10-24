#!/usr/bin/env python3
"""verify-not-shipped.py

Filters out PRs that show up as "new" between OLD_TAG and NEW_TAG by commit
ancestry, but whose code is already present at OLD_TAG.

This happens when a release branch's history gets reconciled out of band --
e.g. a one-off commit squashes several PRs' changes into the release line
early (under a new SHA) during RC staging, and the *original* per-PR commits
get merged into the release branch separately, later, as a pure content
no-op. `git merge-base --is-ancestor <original-commit> OLD_TAG` then says
"not an ancestor" even though the code has been there since OLD_TAG, which
fools a pure commit-ancestry diff like prs-between-tags.sh's. See this
skill's "Known failure mode" section in SKILL.md for the incident that
motivated this script.

Reads the PR list from stdin in the format prs-between-tags.sh emits (one
header line per PR, then its changed files indented four spaces), and for
each PR runs two mechanical checks against a local clone:

  1. Zero-diff check (cheap, no API calls): if every one of the PR's files
     is byte-identical between OLD_TAG and NEW_TAG, the PR cannot possibly
     represent new code in this range, full stop. -> DROP.
  2. Added-line-run containment check (one extra `gh` call per surviving
     PR, to resolve its merge commit): for each contiguous run of lines the
     PR's own commit added, check whether that exact, ordered run already
     appears in OLD_TAG's version of the file. Isolated single-line runs are
     only counted if the line is long/specific enough to not be coincidental
     ("}" or "return nil" show up constantly in any large Go file; a run of
     2+ consecutive added lines, or one long distinctive line, does not).
     This is deliberately looser than requiring the hunk's surrounding
     *context* to match too: when several PRs from the same squash batch
     land in the same file back-to-back, the context immediately around any
     one PR's hunk can shift from sibling edits even though that PR's own
     added lines are still sitting there untouched. An earlier version of
     this script required context match and undercounted real matches for
     exactly that reason; a version before *that* matched bare individual
     lines with no distinctiveness filter and over-matched on common Go
     idioms (flagged PR #227 and #1944 in the incident that motivated this
     script, neither of which had anything to do with the squash commit).

     A file can legitimately keep changing after a PR lands (other PRs
     touch it too), so a nonzero whole-file diff doesn't clear a PR by
     itself - only a low containment fraction does.

Verdicts (one per line, to stdout):
  DROP <num>  <reason>   - zero-diff: safe to exclude automatically.
  FLAG <num>  <reason>   - high/partial hunk containment: likely or possibly
                           already shipped. Spot-check before excluding; do
                           not drop on this signal alone.
  KEEP <num>  <reason>   - low containment, or nothing to check: treat as
                           genuinely new, same as before this script existed.

Usage:
  ./scripts/prs-between-tags.sh OLD_TAG NEW_TAG > /tmp/prs.txt
  ./scripts/verify-not-shipped.py OLD_TAG NEW_TAG [owner/repo] \
      < /tmp/prs.txt > /tmp/verdicts.txt

Must be run from inside a local clone of the repo with both tags fetched
(git fetch origin --tags). gh is only needed for PRs that reach the
containment check (i.e. did not already zero-diff out). Only the standard
library is used - no pip installs required.
"""
import re
import subprocess
import sys

FLAG_HIGH = 80                # >= this % containment -> "likely already shipped"
FLAG_LOW = 30                 # >= this % (below FLAG_HIGH) -> "partially shipped"

HEADER_RE = re.compile(r"^#(\d+)")
FILE_RE = re.compile(r"^[ \t]+(.+)$")
HUNK_HEADER_RE = re.compile(r"^@@ .* @@")


def run(args, **kw):
    # errors="replace": some touched files are binary (.der/.pem/.png/.snap
    # etc.) - decode them lossily rather than crashing the whole run on one
    # PR that happens to touch a non-UTF-8 blob.
    return subprocess.run(args, capture_output=True, text=True, errors="replace", **kw)


def git(*args):
    r = run(["git", *args])
    return r.returncode, r.stdout, r.stderr


def parse_stdin(fh):
    """Yields (pr_number, [files]) in the order prs-between-tags.sh emits them."""
    num = None
    files: list[str] = []
    for line in fh:
        line = line.rstrip("\n")
        m = HEADER_RE.match(line)
        if m:
            if num is not None:
                yield num, files
            num = m.group(1)
            files = []
            continue
        if num is not None:
            fm = FILE_RE.match(line)
            if fm:
                files.append(fm.group(1).strip())
    if num is not None:
        yield num, files


def zero_diff(old_tag: str, new_tag: str, files: list[str]) -> bool:
    if not files:
        return False
    rc, out, _ = git("diff", "--numstat", old_tag, new_tag, "--", *files)
    if rc != 0:
        return False
    total = 0
    for line in out.splitlines():
        parts = line.split("\t")
        if len(parts) < 2:
            continue
        for p in parts[:2]:
            if p.strip().isdigit():
                total += int(p)
    return total == 0


def merge_commit_sha(num: str, repo: str) -> str | None:
    r = run(["gh", "pr", "view", num, "--repo", repo, "--json", "mergeCommit",
             "--jq", ".mergeCommit.oid"])
    sha = r.stdout.strip()
    return sha or None


def file_lines_at(ref: str, path: str) -> list[str] | None:
    rc, out, _ = git("show", f"{ref}:{path}")
    if rc != 0:
        return None
    return out.split("\n")


def added_runs(old_rev: str, new_rev: str, path: str) -> list[list[str]]:
    """Returns one list-of-lines per contiguous run of added ('+') lines in
    the diff. Context lines are deliberately excluded: when several PRs from
    the same squash batch land in the same file back-to-back, the context
    immediately around any one PR's hunk can shift (sibling edits move
    things), even though that PR's own added lines are still sitting there
    untouched. Requiring context to match as well, as an earlier version of
    this script did, undercounted real matches for exactly that reason."""
    rc, out, _ = git("diff", "-U3", old_rev, new_rev, "--", path)
    if rc != 0 or not out:
        return []
    runs: list[list[str]] = []
    current: list[str] = []
    in_hunk = False
    for line in out.splitlines():
        if HUNK_HEADER_RE.match(line):
            in_hunk = True
            continue
        if not in_hunk:
            continue
        if line.startswith("+"):
            current.append(line[1:])
            continue
        if current:
            runs.append(current)
            current = []
    if current:
        runs.append(current)
    return runs


def run_is_distinctive(run: list[str]) -> bool:
    """A run of added lines is worth checking only if it's unlikely to match
    by coincidence. A single short line ("}", "return nil") is common enough
    to show up anywhere in a large file; a run of 2+ consecutive added lines,
    or one long enough line, is much more specific."""
    stripped = [l.strip() for l in run if l.strip()]
    if not stripped:
        return False
    if len(stripped) >= 2:
        return True
    return len("".join(stripped[0].split())) >= 16  # one line, but a long one


def block_contained(block: list[str], haystack: list[str]) -> bool:
    """Exact, order-preserved, contiguous-subsequence match, ignoring
    trailing whitespace and lines that went blank after stripping."""
    needle = [l.rstrip() for l in block if l.strip()]
    if not needle:
        return False
    hay = [l.rstrip() for l in haystack]
    n, m = len(needle), len(hay)
    if n > m:
        return False
    for start in range(m - n + 1):
        if hay[start:start + n] == needle:
            return True
    return False


def check_pr(num: str, files: list[str], old_tag: str, new_tag: str, repo: str) -> str:
    if not files:
        return f"KEEP {num} (no files listed, cannot verify)"

    if zero_diff(old_tag, new_tag, files):
        return (f"DROP {num} (zero-diff: all changed files are byte-identical "
                f"between {old_tag} and {new_tag})")

    sha = merge_commit_sha(num, repo)
    if not sha:
        return f"KEEP {num} (could not resolve a merge commit for the containment check)"
    rc, _, _ = git("rev-parse", f"{sha}^")
    if rc != 0:
        return f"KEEP {num} (merge commit {sha[:12]} has no resolvable parent locally)"

    runs_total = 0
    runs_contained = 0
    for f in files:
        old_content = file_lines_at(old_tag, f)
        if old_content is None:
            continue  # file didn't exist at OLD_TAG - can't be already shipped
        for run in added_runs(f"{sha}^", sha, f):
            if not run_is_distinctive(run):
                continue
            runs_total += 1
            if block_contained(run, old_content):
                runs_contained += 1

    if runs_total == 0:
        return f"KEEP {num} (no distinctive added lines to check, or files missing at one tag)"

    pct = round(100 * runs_contained / runs_total)
    if pct >= FLAG_HIGH:
        return (f"FLAG {num} ({pct}% of its own added-line runs already present in "
                f"{old_tag} - likely already shipped, confirm before dropping)")
    if pct >= FLAG_LOW:
        return (f"FLAG {num} ({pct}% of its own added-line runs already present in "
                f"{old_tag} - partially shipped, spot-check)")
    return (f"KEEP {num} ({pct}% of its own added-line runs already present in "
            f"{old_tag} - looks genuinely new)")


def main() -> int:
    if len(sys.argv) < 3 or len(sys.argv) > 4:
        print(f"Usage: {sys.argv[0]} <old-tag> <new-tag> [owner/repo] < prs.txt",
              file=sys.stderr)
        return 1
    old_tag, new_tag = sys.argv[1], sys.argv[2]
    repo = sys.argv[3] if len(sys.argv) == 4 else "gravitational/core"

    for tag in (old_tag, new_tag):
        rc, _, _ = git("rev-parse", tag)
        if rc != 0:
            print(f"error: '{tag}' not found locally - run 'git fetch origin --tags' first",
                  file=sys.stderr)
            return 1

    for num, files in parse_stdin(sys.stdin):
        print(check_pr(num, files, old_tag, new_tag, repo))
    return 0


if __name__ == "__main__":
    sys.exit(main())
