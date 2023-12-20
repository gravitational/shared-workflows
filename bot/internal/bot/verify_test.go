package bot

import (
	"context"
	"strconv"
	"testing"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/stretchr/testify/require"
)

func TestParseMigrationFileID(t *testing.T) {
	cases := []struct {
		file      string
		expect    int
		expecterr bool
	}{
		{expecterr: true},              // 0
		{file: "a", expecterr: true},   // 1
		{file: "a_a", expecterr: true}, // 2
		{file: strconv.Itoa(minMigrationFileID) + "_", expect: minMigrationFileID}, // 3
		{file: "202301031500_subscription-alter.up.sql", expect: 202301031500},     // 4
	}
	for i, test := range cases {
		got, err := parseMigrationFileID(test.file)
		if test.expecterr == (err == nil) {
			if test.expecterr {
				t.Fatalf("[%d] expected error", i)
			} else {
				t.Fatalf("[%d] unexpected error: %v", i, err)
			}
		}
		if got != test.expect {
			t.Errorf("[%d] expected %d got %d", i, test.expect, got)
		}
	}
}

func TestParseMigrationFileIDs(t *testing.T) {
	testFiles := func(path string, prefixes ...int) []string {
		s := make([]string, len(prefixes))
		for i := range prefixes {
			s[i] = path + strconv.Itoa(prefixes[i]) + "_"
		}
		return s
	}

	cases := []struct {
		path      string
		files     []string
		expect    []int
		expecterr bool
	}{
		{ // 0   no files no results or error
			expecterr: false,
		},
		{ // 1   without path
			path:   "",
			files:  testFiles("", minMigrationFileID),
			expect: []int{minMigrationFileID},
		},
		{ // 2   with path
			path:   "a",
			files:  testFiles("a/", minMigrationFileID),
			expect: []int{minMigrationFileID},
		},
		{ // 3   multiple unordered files returns ordered timestamps
			path:   "a",
			files:  testFiles("a/", minMigrationFileID+1, minMigrationFileID),
			expect: []int{minMigrationFileID, minMigrationFileID + 1},
		},
		{ // 4   no timestamp file prefix returns an error
			path:      "a",
			files:     []string{"a/_"},
			expecterr: true,
		},
	}
	for i, test := range cases {
		got, err := parseMigrationFileIDs(test.path, test.files)
		if test.expecterr == (err == nil) {
			if test.expecterr {
				t.Fatalf("[%d] expected error", i)
			} else {
				t.Fatalf("[%d] unexpected error: %v", i, err)
			}
		}
		if len(got) != len(test.expect) {
			t.Fatalf("[%d] expected %d ids got %d", i, len(test.expect), len(got))
		}
		for j := range got {
			if got[j] != test.expect[j] {
				t.Errorf("[%d:%d] expected %d got %d", i, j, test.expect[j], got[j])
			}
		}
	}
}

func TestVerifyCloudDBMigration(t *testing.T) {
	const defaultStatus = "added"

	// load fake github with noop data
	fgh := &fakeGithub{ref: github.Reference{Name: "foo", SHA: "abc"}}
	fgh.files = []github.PullRequestFile{{Name: "foo.go"}, {Name: "pkg/lib/foo.go"}}
	fgh.commitFiles = []string{"foo.go", "pkg/lib/foo.go", "README.md"}
	bot, err := New(&Config{
		GitHub: fgh,
		Environment: &env.Environment{
			UnsafeBase: "master",
		},
	})
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}

	cases := []struct {
		prFiles     []string
		branchFiles []string
		status      string
		expectErr   bool
	}{
		{}, // 0    no migration files in branch or pr
		{ // 1 OK   no migration files in base branch
			prFiles: []string{
				"db/202301031501_adding.up.sql",
			},
		},
		{ // 2 OK   no migration files in PR
			branchFiles: []string{
				"db/202301031500_exists.up.sql",
			},
		},
		{ // 3 OK   migration files in PR are newer than base branch
			prFiles: []string{
				"db/202301031501_adding.up.sql",
				"db/202301031501_adding.down.sql",
			},
			branchFiles: []string{
				"db/202301031500_exists.up.sql",
			},
		},
		{ // 4 FAIL  migration files in PR are older than base branch
			prFiles: []string{
				"db/202301031500_adding.up.sql",
				"db/202301031500_adding.down.sql",
			},
			branchFiles: []string{
				"db/202301031501_exists.up.sql",
			},
			expectErr: true,
		},
		{ // 5 OK   filter removed files
			status: "removed",
			prFiles: []string{
				"db/202301031501_fake.up.sql",
			},
			branchFiles: []string{
				"db/202301031501_exists.up.sql",
			},
		},
		{ // 6 OK extra down file
			prFiles: []string{
				"db/202301031501_adding.up.sql",
				"db/202301031501_adding.down.sql",
				"db/202109104352_exclude.down.sql",
			},
			branchFiles: []string{
				"db/202301031500_exists.up.sql",
			},
		},
	}
	fghBaseline := *fgh
	for i, test := range cases {
		if test.status == "" {
			test.status = defaultStatus
		}
		for _, f := range test.prFiles {
			fgh.files = append(fgh.files, github.PullRequestFile{
				Name:   f,
				Status: test.status,
			})
		}
		fgh.commitFiles = append(fgh.commitFiles, test.branchFiles...)

		err = bot.verifyDBMigration(context.Background(), "db")
		if err != nil {
			if test.expectErr == (err == nil) {
				if test.expectErr {
					t.Fatalf("[%d] expected error", i)
				} else {
					t.Fatalf("[%d] unexpected error: %v", i, err)
				}
			}
		}

		*fgh = fghBaseline
	}
}

func TestFilterDownMigrationFiles(t *testing.T) {
	cases := []struct {
		names  []string
		expect []string
	}{
		{
			names:  []string{"foo", "bar.up.sql", "bar.down.sql"},
			expect: []string{"foo", "bar.up.sql"},
		},
	}
	for _, test := range cases {
		got := excludeDownMigrationFiles(test.names)
		require.Equal(t, test.expect, got)
	}
}
