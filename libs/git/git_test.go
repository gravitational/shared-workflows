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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepo initialises a temporary git repository for use in tests.
func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	repo := NewRepo(t.TempDir())

	mustGit := func(args ...string) {
		t.Helper()
		_, err := repo.RunCmd(args...)
		require.NoError(t, err, "git %v failed", args)
	}

	mustGit("init", "-b", "main")
	mustGit("config", "user.email", "test@example.com")
	mustGit("config", "user.name", "Test")
	mustGit("config", "tag.gpgSign", "false")
	mustGit("config", "commit.gpgSign", "false")

	return repo
}

// addCommit writes a unique file and creates a commit with the given message.
func addCommit(t *testing.T, repo *Repo, msg string) {
	t.Helper()
	f := filepath.Join(repo.dir, msg+".txt")
	require.NoError(t, os.WriteFile(f, []byte(msg), 0o644))
	_, err := repo.RunCmd("add", ".")
	require.NoError(t, err)
	_, err = repo.RunCmd("commit", "-m", msg)
	require.NoError(t, err)
}

func TestPRsBetweenRefs(t *testing.T) {
	tests := []struct {
		name     string
		subjects []string // commit subjects after the tag, oldest first
		want     []int
	}{
		{
			name:     "all commits reference PRs",
			subjects: []string{"fix something (#101)", "add feature (#102)", "fix bug (#103)"},
			want:     []int{103, 102, 101},
		},
		{
			name:     "commits without a PR are skipped",
			subjects: []string{"fix something (#101)", "chore: update deps", "fix bug (#103)"},
			want:     []int{103, 101},
		},
		{
			name:     "duplicate PRs are returned once",
			subjects: []string{"fix something (#101)", "follow-up fix (#102)", "backport note (#101)"},
			want:     []int{101, 102},
		},
		{
			name:     "empty range",
			subjects: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newTestRepo(t)
			addCommit(t, repo, "initial commit (#99)")
			_, err := repo.RunCmd("tag", "v1.0.0")
			require.NoError(t, err)
			for _, subject := range tt.subjects {
				addCommit(t, repo, subject)
			}

			prs, err := repo.PRsBetweenRefs("v1.0.0", "HEAD")
			require.NoError(t, err)
			assert.Equal(t, tt.want, prs)
		})
	}
}

func TestPRsBetweenRefs_FirstParentOnly(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit (#99)")
	_, err := repo.RunCmd("tag", "v1.0.0")
	require.NoError(t, err)

	addCommit(t, repo, "mainline fix (#101)")
	_, err = repo.RunCmd("checkout", "-b", "topic")
	require.NoError(t, err)
	addCommit(t, repo, "topic-only change (#999)")
	_, err = repo.RunCmd("checkout", "main")
	require.NoError(t, err)
	addCommit(t, repo, "mainline feature (#102)")
	_, err = repo.RunCmd("merge", "--no-ff", "topic", "-m", "Merge topic")
	require.NoError(t, err)

	prs, err := repo.PRsBetweenRefs("v1.0.0", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, []int{102, 101}, prs)
}

func TestObjectSHAAtPath(t *testing.T) {
	repo := newTestRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "sub.txt"), []byte("content"), 0o644))
	_, err := repo.RunCmd("add", "sub.txt")
	require.NoError(t, err)
	_, err = repo.RunCmd("commit", "-m", "add sub.txt")
	require.NoError(t, err)

	// A blob's SHA is determined by its content alone: this is
	// `git hash-object` of "content", and notably not the commit SHA.
	got, err := repo.ObjectSHAAtPath("HEAD", "sub.txt")
	require.NoError(t, err)
	assert.Equal(t, "6b584e8ece562ebffc15d38808cd6b98fc3d97ea", got)
}

func TestObjectSHAAtPath_InvalidRef(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial")

	_, err := repo.ObjectSHAAtPath("nonexistent-ref", "somefile")
	assert.Error(t, err)
}

// addCommitWithBody creates a commit with the given subject and body.
func addCommitWithBody(t *testing.T, repo *Repo, subject, body string) {
	t.Helper()
	f := filepath.Join(repo.dir, strings.ReplaceAll(subject, "/", "_")+".txt")
	require.NoError(t, os.WriteFile(f, []byte(subject), 0o644))
	_, err := repo.RunCmd("add", ".")
	require.NoError(t, err)
	_, err = repo.RunCmd("commit", "-m", subject, "-m", body)
	require.NoError(t, err)
}

func TestCommitPRNumber(t *testing.T) {
	tests := []struct {
		subject string
		want    int
	}{
		{subject: "fix something (#101)", want: 101},
		{subject: "fix something (#101) ", want: 101},
		{subject: "chore: update deps", want: 0},
		{subject: "mentions (#101) mid-subject", want: 0},
		// Backports accumulate refs; the merge number is the trailing one.
		{subject: "fix something (#101) (#202)", want: 202},
		{subject: "issue #101", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			assert.Equal(t, tt.want, Commit{Subject: tt.subject}.PRNumber())
		})
	}
}

func TestCommitsInRange(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit (#99)")
	_, err := repo.RunCmd("tag", "v1.0.0")
	require.NoError(t, err)
	addCommitWithBody(t, repo, "squash merge (#101)", "* first squashed (#11)\n* second squashed (#12)")

	commits, err := repo.CommitsInRange("v1.0.0..HEAD", false)
	require.NoError(t, err)
	require.Len(t, commits, 1)

	assert.Equal(t, "squash merge (#101)", commits[0].Subject)
	assert.Equal(t, 101, commits[0].PRNumber())
	// Multiline bodies must survive intact: they are the only local record of
	// the commits a squash merge collapsed.
	assert.Equal(t, "* first squashed (#11)\n* second squashed (#12)", strings.TrimSpace(commits[0].Body))
	assert.Len(t, commits[0].SHA, 40)
	assert.False(t, commits[0].Date.IsZero())
}

func TestCommitsInRange_FirstParentOnly(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit (#99)")
	_, err := repo.RunCmd("tag", "v1.0.0")
	require.NoError(t, err)
	addCommit(t, repo, "mainline fix (#101)")
	_, err = repo.RunCmd("checkout", "-b", "topic")
	require.NoError(t, err)
	addCommit(t, repo, "topic-only change (#999)")
	_, err = repo.RunCmd("checkout", "main")
	require.NoError(t, err)
	_, err = repo.RunCmd("merge", "--no-ff", "topic", "-m", "Merge topic (#102)")
	require.NoError(t, err)

	firstParent, err := repo.CommitsInRange("v1.0.0..HEAD", true)
	require.NoError(t, err)
	assert.Equal(t, []int{102, 101}, prNumbers(firstParent))

	// Cherry-picks and merged-in work sit off the first-parent chain, so the
	// released set needs the full traversal to see them.
	all, err := repo.CommitsInRange("v1.0.0..HEAD", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int{102, 101, 999}, prNumbers(all))
}

func TestCommitsInRange_Empty(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit (#99)")

	commits, err := repo.CommitsInRange("HEAD..HEAD", false)
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestCommitsInRange_InvalidRange(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial")

	_, err := repo.CommitsInRange("nonexistent-ref..HEAD", false)
	assert.Error(t, err)
}

func prNumbers(commits []Commit) []int {
	numbers := make([]int, 0, len(commits))
	for _, commit := range commits {
		numbers = append(numbers, commit.PRNumber())
	}
	return numbers
}

func TestMergeBase(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "shared base")
	base, err := repo.RunCmd("rev-parse", "HEAD")
	require.NoError(t, err)

	_, err = repo.RunCmd("checkout", "-b", "release")
	require.NoError(t, err)
	addCommit(t, repo, "release-only change")
	_, err = repo.RunCmd("checkout", "main")
	require.NoError(t, err)
	addCommit(t, repo, "mainline change")

	got, err := repo.MergeBase("release", "main")
	require.NoError(t, err)
	assert.Equal(t, base, got)
}

func TestMergeBase_AncestorIsItsOwnMergeBase(t *testing.T) {
	// This is the case the released set relies on to stay empty: when the base
	// tag is an ancestor of the head branch, the chain range is empty and the
	// plain commit range is already correct.
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit")
	_, err := repo.RunCmd("tag", "v1.0.0")
	require.NoError(t, err)
	tag, err := repo.RunCmd("rev-parse", "v1.0.0")
	require.NoError(t, err)
	addCommit(t, repo, "later commit")

	got, err := repo.MergeBase("v1.0.0", "HEAD")
	require.NoError(t, err)
	assert.Equal(t, tag, got)

	commits, err := repo.CommitsInRange(got+"..v1.0.0", false)
	require.NoError(t, err)
	assert.Empty(t, commits)
}

func TestMergeBase_UnrelatedHistories(t *testing.T) {
	repo := newTestRepo(t)
	addCommit(t, repo, "initial commit")
	_, err := repo.RunCmd("checkout", "--orphan", "unrelated")
	require.NoError(t, err)
	addCommit(t, repo, "unrelated root")

	_, err = repo.MergeBase("main", "unrelated")
	assert.Error(t, err)
}
