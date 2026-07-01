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
