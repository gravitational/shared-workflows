/*
Copyright 2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package git

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// Repo wraps a local git repository and runs git commands against it.
type Repo struct {
	dir string
}

// NewRepo initializes [Repo] from a directory.
func NewRepo(dir string) *Repo {
	return &Repo{dir: dir}
}

// ObjectSHAAtPath returns the SHA of the object at path as of ref,
// e.g. the commit a submodule points to.
func (r *Repo) ObjectSHAAtPath(ref, path string) (string, error) {
	sha, err := r.RunCmd("rev-parse", ref+":"+path)
	if err != nil {
		return "", trace.Wrap(err, "can't get object SHA for ref %q, path %q", ref, path)
	}
	return sha, nil
}

// prRegex matches the "(#N)" suffix GitHub squash merges append to commit subjects.
var prRegex = regexp.MustCompile(`\(#(\d+)\)\s*$`)

// PRsBetweenRefs returns the unique pull request numbers referenced by
// first-parent commits in baseRef..headRef. A commit references a PR when its
// subject ends with "(#N)", as produced by GitHub squash merges; commits
// without a PR reference are skipped.
func (r *Repo) PRsBetweenRefs(baseRef, headRef string) ([]int, error) {
	commits, err := r.CommitLinesBetweenRefs(baseRef, headRef)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	var prNumbers []int
	seen := make(map[int]struct{})
	for _, commit := range commits {
		matches := prRegex.FindStringSubmatch(commit)
		if matches == nil {
			continue
		}
		n, err := strconv.Atoi(matches[1])
		if err != nil {
			continue // digits too long to be a PR number
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		prNumbers = append(prNumbers, n)
	}

	return prNumbers, nil
}

// CommitLinesBetweenRefs returns abbreviated hashes and subjects for first-parent
// commits in baseRef..headRef.
func (r *Repo) CommitLinesBetweenRefs(baseRef, headRef string) ([]string, error) {
	commits, err := r.RunCmd("log", "--first-parent", "--format=%h %s", baseRef+".."+headRef)
	if err != nil {
		return nil, trace.Wrap(err, "can't get commits between refs %q and %q", baseRef, headRef)
	}
	if commits == "" {
		return nil, nil
	}
	return strings.Split(commits, "\n"), nil
}

// Commit is a single commit's metadata.
type Commit struct {
	// SHA is the full commit hash.
	SHA string
	// Subject is the first line of the commit message.
	Subject string
	// Body is the commit message after the subject. GitHub squash merges put
	// the squashed commits' subjects here as "* <subject> (#N)" bullets.
	Body string
	// Date is the commit date.
	Date time.Time
}

// PRNumber returns the pull request number from the commit subject's "(#N)"
// suffix, as produced by GitHub squash merges. It returns 0 when the subject
// carries no PR reference.
//
// Note that the number alone is not a unique identity: repositories that were
// renamed or forked privately restart PR numbering, so the same number can
// refer to different pull requests. Callers that compare commits across refs
// need to account for that.
func (c Commit) PRNumber() int {
	matches := prRegex.FindStringSubmatch(c.Subject)
	if matches == nil {
		return 0
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0 // digits too long to be a PR number
	}
	return n
}

// commitFieldSep and commitRecordSep delimit fields and records in the
// git log --format output. Both are bytes that cannot appear in a commit
// message, so neither multiline bodies nor subjects can split a record.
//
// They are requested as %x00 and %x1e rather than written into the format
// string directly: the format is an exec argument, and a literal NUL cannot
// be passed as one.
const (
	commitFieldSep        = "\x00"
	commitRecordSep       = "\x1e"
	commitFieldSepFormat  = "%x00"
	commitRecordSepFormat = "%x1e"
)

// CommitsInRange returns the commits in revRange, which is any revision range
// git log accepts, e.g. "baseRef..headRef". When firstParentOnly is set the
// traversal follows only first parents, matching [Repo.CommitLinesBetweenRefs];
// otherwise merged-in commits are included too, which is what callers need when
// a commit may have landed off the first-parent chain, e.g. a cherry-pick.
func (r *Repo) CommitsInRange(revRange string, firstParentOnly bool) ([]Commit, error) {
	format := strings.Join([]string{"%H", "%s", "%b", "%cI"}, commitFieldSepFormat) + commitRecordSepFormat
	args := []string{"log", "--format=" + format}
	if firstParentOnly {
		args = append(args, "--first-parent")
	}
	args = append(args, revRange)

	out, err := r.RunCmd(args...)
	if err != nil {
		return nil, trace.Wrap(err, "can't get commits in range %q", revRange)
	}

	var commits []Commit
	for _, record := range strings.Split(out, commitRecordSep) {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.Split(record, commitFieldSep)
		if len(fields) != 4 {
			return nil, trace.BadParameter("malformed git log record in range %q: %q", revRange, record)
		}
		date, err := time.Parse(time.RFC3339, fields[3])
		if err != nil {
			return nil, trace.Wrap(err, "can't parse commit date %q", fields[3])
		}
		commits = append(commits, Commit{
			SHA:     fields[0],
			Subject: fields[1],
			Body:    fields[2],
			Date:    date,
		})
	}

	return commits, nil
}

// MergeBase returns the best common ancestor of the two refs. When one ref is
// an ancestor of the other, the ancestor itself is returned.
func (r *Repo) MergeBase(ref1, ref2 string) (string, error) {
	sha, err := r.RunCmd("merge-base", ref1, ref2)
	if err != nil {
		return "", trace.Wrap(err, "can't get merge base of refs %q and %q", ref1, ref2)
	}
	return sha, nil
}
