package planfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func samplePlan() *Plan {
	return &Plan{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
		AccountID:     "123456789012",
		Action:        "deprecate",
		DryRun:        false,
		Entries: []PlanEntry{
			{Region: "us-east-1", ImageID: "ami-1", Status: models.StatusPending},
			{Region: "us-east-1", ImageID: "ami-2", Status: models.StatusPending},
			{Region: "eu-west-1", ImageID: "ami-3", Status: models.StatusPending},
		},
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	want := samplePlan()
	require.NoError(t, Save(want, path))

	got, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, want.SchemaVersion, got.SchemaVersion)
	assert.Equal(t, want.AccountID, got.AccountID)
	assert.Equal(t, want.Action, got.Action)
	assert.Len(t, got.Entries, len(want.Entries))
	for i := range want.Entries {
		assert.Equal(t, want.Entries[i].ImageID, got.Entries[i].ImageID)
		assert.Equal(t, want.Entries[i].Status, got.Entries[i].Status)
	}
}

func TestSave_AtomicReplaceOnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	require.NoError(t, Save(samplePlan(), path))

	updated := samplePlan()
	updated.Entries[0].Status = models.StatusCompleted
	require.NoError(t, Save(updated, path))

	got, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, models.StatusCompleted, got.Entries[0].Status)

	_, err = os.Stat(path + ".tmp")
	assert.True(t, errors.Is(err, os.ErrNotExist), "tmp file should be cleaned up; got %v", err)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	require.Error(t, err)
}

func TestLoad_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_WrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	p := samplePlan()
	p.SchemaVersion = 999
	require.NoError(t, Save(p, path))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema_version")
}

func TestPlanStore_Mark_PersistsToLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)

	require.NoError(t, store.Mark(0, models.StatusCompleted, nil))
	require.NoError(t, store.Mark(1, models.StatusFailed, errors.New("rate limit")))
	require.NoError(t, store.Close())

	// Reopen the store; replay should restore the marks.
	reopened, err := OpenStore(path)
	require.NoError(t, err)
	defer reopened.Close()

	rp := reopened.Plan()
	assert.Equal(t, models.StatusCompleted, rp.Entries[0].Status)
	require.NotNil(t, rp.Entries[0].CompletedAt)
	assert.Equal(t, models.StatusFailed, rp.Entries[1].Status)
	assert.Equal(t, "rate limit", rp.Entries[1].Error)
	assert.Equal(t, models.StatusPending, rp.Entries[2].Status, "untouched entries stay pending")
}

func TestPlanStore_Mark_PlanFileNotRewritten(t *testing.T) {
	// Critical for write-amplification: after Mark, the plan file's mtime/size
	// must not change. Status updates go to the .log sibling instead.
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)

	planInfoBefore, err := os.Stat(path)
	require.NoError(t, err)
	logPath := path + ".log"
	logInfoBefore, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Zero(t, logInfoBefore.Size(), "log should start empty")

	for i := 0; i < 100; i++ {
		require.NoError(t, store.Mark(i%3, models.StatusCompleted, nil))
	}
	require.NoError(t, store.Close())

	planInfoAfter, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, planInfoBefore.Size(), planInfoAfter.Size(), "plan file size must not change across Marks")
	assert.Equal(t, planInfoBefore.ModTime(), planInfoAfter.ModTime(), "plan file must not be rewritten across Marks")

	logInfoAfter, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Greater(t, logInfoAfter.Size(), int64(0), "log should contain the appended records")
}

func TestPlanStore_Pending_ReturnsNonCompletedIndices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	p := samplePlan()
	p.Entries[0].Status = models.StatusCompleted
	p.Entries[1].Status = models.StatusFailed // failed counts as pending for retry
	p.Entries[2].Status = models.StatusPending
	require.NoError(t, Save(p, path))

	store, err := OpenStore(path)
	require.NoError(t, err)
	defer store.Close()
	assert.ElementsMatch(t, []int{1, 2}, store.Pending())
}

func TestPlanStore_Mark_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	defer store.Close()

	require.Error(t, store.Mark(99, models.StatusCompleted, nil))
}

func TestPlanStore_Mark_AfterCloseFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	require.NoError(t, store.Close())

	err = store.Mark(0, models.StatusCompleted, nil)
	require.Error(t, err)
}

func TestPlanStore_Mark_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	p := samplePlan()
	for i := 0; i < 50; i++ {
		p.Entries = append(p.Entries, PlanEntry{Region: "us-east-1", ImageID: "ami-x", Status: models.StatusPending})
	}

	store, err := NewStore(path, p)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < len(p.Entries); i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			require.NoError(t, store.Mark(idx, models.StatusCompleted, nil))
		}(i)
	}
	wg.Wait()
	require.NoError(t, store.Close())

	reopened, err := OpenStore(path)
	require.NoError(t, err)
	defer reopened.Close()
	for i, e := range reopened.Plan().Entries {
		assert.Equal(t, models.StatusCompleted, e.Status, "entry %d not marked", i)
	}
}

func TestPlanStore_Plan_ReturnsCurrentPlan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	defer store.Close()

	require.NoError(t, store.Mark(0, models.StatusCompleted, nil))

	got := store.Plan()
	require.NotNil(t, got)
	assert.Equal(t, models.StatusCompleted, got.Entries[0].Status)
}

func TestNewStore_WritesPlanAndEmptyLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	defer store.Close()

	_, err = os.Stat(path)
	require.NoError(t, err)
	logInfo, err := os.Stat(path + ".log")
	require.NoError(t, err)
	assert.Zero(t, logInfo.Size())
}

func TestNewStore_TruncatesStaleLog(t *testing.T) {
	// If the user re-runs with --plan-file pointing at a path that had a log
	// from a previous attempt, NewStore (which is invoked when the plan file
	// did not exist) should not pick up the old log records.
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	require.NoError(t, os.WriteFile(path+".log", []byte(`{"idx":0,"status":"completed","completed_at":"2026-01-01T00:00:00Z"}`+"\n"), 0o600))

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	defer store.Close()

	logInfo, err := os.Stat(path + ".log")
	require.NoError(t, err)
	assert.Zero(t, logInfo.Size(), "NewStore must truncate any stale log")
}

func TestOpenStore_ReplaysLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	store, err := NewStore(path, samplePlan())
	require.NoError(t, err)
	require.NoError(t, store.Mark(0, models.StatusCompleted, nil))
	require.NoError(t, store.Mark(2, models.StatusFailed, errors.New("api throttled")))
	require.NoError(t, store.Close())

	reopened, err := OpenStore(path)
	require.NoError(t, err)
	defer reopened.Close()

	p := reopened.Plan()
	assert.Equal(t, models.StatusCompleted, p.Entries[0].Status)
	assert.Equal(t, models.StatusPending, p.Entries[1].Status)
	assert.Equal(t, models.StatusFailed, p.Entries[2].Status)
	assert.Equal(t, "api throttled", p.Entries[2].Error)
	assert.ElementsMatch(t, []int{1, 2}, reopened.Pending())
}

func TestOpenStore_TolerantOfTrailingPartialLine(t *testing.T) {
	// Simulates a process that crashed mid-Write: the last log line is
	// truncated. Replay should stop at the bad line and the affected entry
	// should fall back to its plan-file status (pending).
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")

	require.NoError(t, Save(samplePlan(), path))
	require.NoError(t, os.WriteFile(path+".log",
		[]byte(`{"idx":0,"status":"completed","completed_at":"2026-04-30T00:00:00Z"}`+"\n"+
			`{"idx":1,"status":"compl`), // truncated
		0o600))

	store, err := OpenStore(path)
	require.NoError(t, err)
	defer store.Close()

	p := store.Plan()
	assert.Equal(t, models.StatusCompleted, p.Entries[0].Status)
	assert.Equal(t, models.StatusPending, p.Entries[1].Status, "truncated record must not be applied")
}

func TestOpenStore_MissingLogIsFine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	require.NoError(t, Save(samplePlan(), path))

	store, err := OpenStore(path)
	require.NoError(t, err)
	defer store.Close()

	for _, e := range store.Plan().Entries {
		assert.Equal(t, models.StatusPending, e.Status)
	}
}
