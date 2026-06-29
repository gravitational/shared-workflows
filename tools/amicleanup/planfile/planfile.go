// Package planfile persists the AMI work-list to disk so a partially-failed
// run can be resumed idempotently.
//
// On-disk format is a pair of files:
//
//   - <path>          — the immutable plan: schema version, account, action,
//     and the list of AMIs to act on. Written once, never
//     updated.
//   - <path>.log      — an append-only JSON-lines status log. Each line is a
//     record of one Mark call: {idx, status, error,
//     completed_at}. New records are appended; nothing is
//     ever rewritten.
//
// Resuming reads <path> for the entry list and replays <path>.log to
// reconstruct each entry's current status. This keeps total bytes-written at
// O(N) for N entries (one short line per Mark) instead of O(N²) (full plan
// rewrite per Mark) — important for accounts with tens of thousands of AMIs.
//
// Concurrency: all writes go through a mutex. Each log line is small and is
// written by a single Write call, so the underlying file ordering is well-
// defined. PlanStore.Close fsyncs and closes the log; if the process crashes
// without Close, recently-written records may be lost from OS cache, and the
// affected entries will simply be retried on resume (idempotent).
package planfile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/shared-workflows/tools/amicleanup/models"
)

const SchemaVersion = 1

// logSuffix is appended to the plan path to derive the status-log path.
const logSuffix = ".log"

type Plan struct {
	SchemaVersion int         `json:"schema_version"`
	GeneratedAt   time.Time   `json:"generated_at"`
	AccountID     string      `json:"account_id"`
	Action        string      `json:"action"`
	DryRun        bool        `json:"dry_run"`
	Entries       []PlanEntry `json:"entries"`
}

type PlanEntry struct {
	Region       string             `json:"region"`
	ImageID      string             `json:"image_id"`
	ImageName    string             `json:"image_name,omitempty"`
	CreationDate time.Time          `json:"creation_date,omitempty"`
	SnapshotIDs  []string           `json:"snapshot_ids,omitempty"`
	Status       models.EntryStatus `json:"status"`
	Error        string             `json:"error,omitempty"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
}

// logRecord is one append-only entry in the status log.
type logRecord struct {
	Idx         int                `json:"idx"`
	Status      models.EntryStatus `json:"status"`
	Error       string             `json:"error,omitempty"`
	CompletedAt time.Time          `json:"completed_at"`
}

// Save serialises plan as JSON and writes it to path atomically (tmp + rename).
// Save does NOT touch the .log sibling — that's the caller's responsibility
// (typically NewStore creates an empty log; OpenStore preserves it).
func Save(plan *Plan, path string) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plan: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create tmp plan: %w", err)
	}

	if _, werr := f.Write(data); werr != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write tmp plan: %w", werr)
	}

	if serr := f.Sync(); serr != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsync tmp plan: %w", serr)
	}

	if cerr := f.Close(); cerr != nil {
		os.Remove(tmp)
		return fmt.Errorf("close tmp plan: %w", cerr)
	}

	if rerr := os.Rename(tmp, path); rerr != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename tmp plan: %w", rerr)
	}

	return nil
}

// Load reads and validates a plan file. It does NOT replay the log; callers
// who want the merged view should use OpenStore instead.
func Load(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}

	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal plan: %w", err)
	}

	if p.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("plan has schema_version=%d, this binary supports %d", p.SchemaVersion, SchemaVersion)
	}

	return &p, nil
}

// PlanStore couples a Plan with its append-only log. Mark mutates the in-memory
// plan and appends a single line to the log; concurrent calls are serialised
// by the internal mutex.
type PlanStore struct {
	planPath string
	logPath  string

	mu   sync.Mutex
	log  *os.File // append-only handle; nil after Close
	plan *Plan
}

// NewStore writes plan to planPath, creates an empty log alongside, and
// returns a store ready for Mark. Use this after enumeration when --plan-file
// is set (or when an ephemeral plan is desired).
func NewStore(planPath string, plan *Plan) (*PlanStore, error) {
	if err := Save(plan, planPath); err != nil {
		return nil, err
	}

	logPath := planPath + logSuffix
	// O_TRUNC: a freshly-created plan should not inherit a stale log from a
	// previous run that happened to use the same path.
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create plan log %s: %w", logPath, err)
	}

	return &PlanStore{
		planPath: planPath,
		logPath:  logPath,
		log:      logFile,
		plan:     plan,
	}, nil
}

// OpenStore loads an existing plan, replays the log to recompute statuses,
// then opens the log for appending. Use this to resume a previous run.
//
// A missing log file is treated as "no completions yet" — that is, every
// entry's Status remains as it was in the plan file. This makes OpenStore
// safe against plan files created by raw Save (e.g. in tests) which do not
// have a log sibling.
func OpenStore(planPath string) (*PlanStore, error) {
	plan, err := Load(planPath)
	if err != nil {
		return nil, err
	}

	logPath := planPath + logSuffix
	if err := replayLog(logPath, plan); err != nil {
		return nil, fmt.Errorf("replay log %s: %w", logPath, err)
	}

	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open plan log %s: %w", logPath, err)
	}

	return &PlanStore{
		planPath: planPath,
		logPath:  logPath,
		log:      logFile,
		plan:     plan,
	}, nil
}

// Plan returns the in-memory plan. Callers must not mutate fields directly;
// use Mark instead.
func (s *PlanStore) Plan() *Plan {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.plan
}

// Mark updates the entry at idx, sets CompletedAt, and appends a single
// JSON-line record to the log. applyErr may be nil on success.
//
// The cost is one short Write (typically <200 bytes) plus the in-memory
// update — independent of the total number of plan entries. We do not fsync
// per call; durability is enforced once on Close. If the process crashes
// before Close, recently-marked entries may be re-applied on resume — every
// supported action is idempotent at the AWS level so that's safe.
func (s *PlanStore) Mark(idx int, status models.EntryStatus, applyErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idx < 0 || idx >= len(s.plan.Entries) {
		return fmt.Errorf("entry index %d out of range [0,%d)", idx, len(s.plan.Entries))
	}

	now := time.Now().UTC()
	rec := logRecord{Idx: idx, Status: status, CompletedAt: now}
	if applyErr != nil {
		rec.Error = applyErr.Error()
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal log record: %w", err)
	}
	line = append(line, '\n')

	if s.log == nil {
		return errors.New("plan store closed")
	}
	if _, err := s.log.Write(line); err != nil {
		return fmt.Errorf("append log: %w", err)
	}

	// Update in-memory state to match what we just persisted.
	s.plan.Entries[idx].Status = status
	s.plan.Entries[idx].CompletedAt = &now
	if applyErr != nil {
		s.plan.Entries[idx].Error = applyErr.Error()
	} else {
		s.plan.Entries[idx].Error = ""
	}

	return nil
}

// Pending returns the indices of entries that still need work — i.e. entries
// whose Status is not Completed. Failed entries are returned for retry.
func (s *PlanStore) Pending() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []int
	for i, e := range s.plan.Entries {
		if e.Status != models.StatusCompleted {
			out = append(out, i)
		}
	}

	return out
}

// Close fsyncs and closes the log. After Close, further Mark calls return an
// error. Safe to call multiple times.
func (s *PlanStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.log == nil {
		return nil
	}
	syncErr := s.log.Sync()
	closeErr := s.log.Close()
	s.log = nil
	if syncErr != nil {
		return fmt.Errorf("fsync log: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close log: %w", closeErr)
	}
	return nil
}

// replayLog reads logPath line-by-line and applies each record to plan.
// Missing log file is fine (no-op). A trailing partial line (e.g. from a
// crash mid-write) is silently skipped — the corresponding entry will retry.
// Records referencing unknown indices are skipped with no error.
func replayLog(logPath string, plan *Plan) error {
	f, err := os.Open(logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Allow lines up to 1 MiB; individual records are far smaller, but error
	// strings can be unbounded.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec logRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			// Last-line truncation or other corruption: stop replay here. The
			// affected entry stays in its prior state and will be retried.
			break
		}
		if rec.Idx < 0 || rec.Idx >= len(plan.Entries) {
			continue
		}
		plan.Entries[rec.Idx].Status = rec.Status
		plan.Entries[rec.Idx].Error = rec.Error
		ca := rec.CompletedAt
		plan.Entries[rec.Idx].CompletedAt = &ca
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
