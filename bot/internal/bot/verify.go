package bot

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gravitational/shared-workflows/bot/internal/env"
	"github.com/gravitational/shared-workflows/bot/internal/github"
	"github.com/gravitational/trace"
)

// Verify is a catch-all for verifying the PR doesn't have any issues.
func (b *Bot) Verify(ctx context.Context) error {
	switch b.c.Environment.Repository {
	case env.CloudRepo:
		err := b.verifyCloud(ctx)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// verifyCloud runs verification checks for cloud repos.
// E.g. it is used to verify DB migration files are ordered properly in the Cloud repo.
func (b *Bot) verifyCloud(ctx context.Context) error {
	// exec DB migration verification
	for _, path := range migrationConfig[b.c.Environment.Repository] {
		err := b.verifyDBMigration(ctx, path)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

// migrationConfig enables the DB migration verification for a repo/path.
//   map[repo]: [...path]
var migrationConfig = map[string][]string{
	env.CloudRepo: []string{"db/salescenter/migrations"},
}

// verifyDBMigration ensures the DB migration files in a PR have a timestamp
// that is more recent than the migration files in the base branch.
func (b *Bot) verifyDBMigration(ctx context.Context, pathPrefix string) error {
	// get all PR files
	prFiles, err := b.c.GitHub.ListFiles(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		b.c.Environment.Number)
	if err != nil {
		return trace.Wrap(err)
	}

	// no files in the PR? okay then.
	if len(prFiles) == 0 {
		log.Print("Verify:cloudDBMigration: no PR files")
		return nil
	}

	// don't evaluate removed files
	prFiles = filterSlice(prFiles, func(f github.PullRequestFile) bool {
		return f.Status != "removed"
	})

	// parse PR migration file ids
	// 202301031500_subscription-alter.up.sql => 202301031500
	prIDs, err := parseMigrationFileIDs(pathPrefix, pullRequestFileNames(prFiles))
	if err != nil {
		return trace.Wrap(err)
	}

	// no PR migration files
	if len(prIDs) == 0 {
		log.Print("Verify:cloudDBMigration: no migration files in this PR")
		return nil
	}

	// get base branch ref
	branchRef, err := b.c.GitHub.GetRef(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		"heads/"+b.c.Environment.UnsafeBase)
	if err != nil {
		return trace.Wrap(err)
	}

	// get base branch migration files
	branchFiles, err := b.c.GitHub.ListCommitFiles(ctx,
		b.c.Environment.Organization,
		b.c.Environment.Repository,
		branchRef.SHA,
		pathPrefix)
	if err != nil {
		if errors.Is(err, github.ErrTruncatedTree) {
			log.Print("Verify:cloudDBMigration: skipping because the tree size is too big")
			return nil
		}
		return trace.Wrap(err)
	}

	// parse base branch migration file ids
	branchIDs, err := parseMigrationFileIDs(pathPrefix, branchFiles)
	if err != nil {
		return trace.Wrap(err)
	}

	// no base branch migration files
	if len(branchIDs) == 0 {
		log.Printf("Verify:cloudDBMigration: no migration files in the base branch: %s", branchRef.Name)
		return nil
	}

	// error if the oldest migration file in the PR has an older timestamp
	// than the most recent migration file in the base branch
	oldestPRID, newestBranchID := prIDs[0], branchIDs[len(branchIDs)-1]
	log.Printf("Verify:cloudDBMigration: comparing migration file IDs; PR:%d <= Branch:%d", oldestPRID, newestBranchID)
	if oldestPRID <= newestBranchID {
		return trace.Errorf("pull request has an older migration (%d) than the most recent migration file in the %s branch (%d); the name of the migration file needs to be changed to be more recent than %[3]d",
			oldestPRID, b.c.Environment.UnsafeBase, newestBranchID)
	}

	return nil
}

// parseMigrationFileIDs parses each file whose path matches pathPrefix returning
// the prefix ID of each file or returns an error if the file does not have an
// integer prefix. The returned IDs are sorted in ascending order.
//
//	  202301031500_subscription-alter.up.sql => 202301031500
func parseMigrationFileIDs(pathPrefix string, files []string) ([]int, error) {
	var ids []int
	for _, file := range files {
		if strings.HasPrefix(file, pathPrefix) {
			_, f := filepath.Split(file)
			id, err := parseMigrationFileID(f)
			if err != nil {
				return nil, trace.BadParameter("failed to parse migration file %q: %v", file, err)
			}
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	return ids, nil
}

// minMigrationFileID is used to validate migration file timestamps
const minMigrationFileID = 200000000000

// parseMigrationFileID returns the ID portion of a Cloud DB migration file.
//
//	202301031500_subscription-alter.up.sql => 202301031500
func parseMigrationFileID(file string) (int, error) {
	x := strings.Index(file, "_")
	if x == -1 {
		return 0, trace.BadParameter("no underscore found")
	}

	id, err := strconv.Atoi(file[:x])
	if err != nil {
		return 0, trace.BadParameter("invalid integer prefix: %v", err)
	}

	if id < minMigrationFileID {
		return 0, trace.BadParameter("integer prefix is not a valid timestamp")
	}

	return id, nil
}

// pullRequestFileNames returns all Name fields from each PullRequestFile.
func pullRequestFileNames(files []github.PullRequestFile) []string {
	names := make([]string, len(files))
	for i := range files {
		names[i] = files[i].Name
	}
	return names
}

// filterSlice filters a slice returning a slice of the original items
// excluding those itens where filterfunc returns false.
func filterSlice[T any](ts []T, filterfunc func(T) bool) []T {
	p := make([]T, 0, len(ts))
	for i := range ts {
		if filterfunc(ts[i]) {
			p = append(p, ts[i])
		}
	}
	return p
}
