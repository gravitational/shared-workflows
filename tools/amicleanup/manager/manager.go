// Package manager orchestrates the two phases of an amicleanup run:
// enumerate (walk regions and AMIs to build a plan) and apply (execute the
// chosen action on each plan entry, persisting status as we go).
//
// The manager is wired with a RegionalClientFactory so production code can
// supply *ec2.Client per-region while tests substitute fakes. Dry-run is
// applied by wrapping the factory's output in client.NewReadOnly; the manager
// itself doesn't know about dry-run beyond the bool it stores into the plan
// for resume validation.
package manager

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shared-workflows/tools/amicleanup/account"
	"github.com/shared-workflows/tools/amicleanup/actions"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/images"
	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/shared-workflows/tools/amicleanup/planfile"
	"github.com/shared-workflows/tools/amicleanup/regions"
)

// bootstrapRegion is the region whose EC2 client the manager calls
// DescribeRegions against. Any opted-in or default region works; we pick
// us-east-1 as a stable choice. Production main.go injects a factory that
// returns a real us-east-1 client for this name; tests inject a fake.
const bootstrapRegion = "us-east-1"

// Config bundles the dependencies and run-mode flags the manager needs.
type Config struct {
	Factory     ec2iface.RegionalClientFactory
	STS         ec2iface.STSAPI
	Action      actions.Action
	DryRun      bool
	Concurrency int
	Log         io.Writer // where progress lines go (typically os.Stderr)
}

// Manager owns the dependencies. It's value-receiver-friendly; nothing here
// holds locks across calls.
type Manager struct {
	cfg Config
}

func New(cfg Config) *Manager {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 8
	}

	if cfg.Log == nil {
		cfg.Log = io.Discard
	}

	return &Manager{cfg: cfg}
}

// Enumerate walks every enabled region, lists owned AMIs in each, and returns
// a Plan ready for Apply. RegionErrors collects per-region failures so the
// caller can decide whether to proceed.
func (m *Manager) Enumerate(ctx context.Context) (*planfile.Plan, []RegionError, error) {
	bootstrap := m.cfg.Factory(bootstrapRegion)

	acct, err := account.CallerAccountID(ctx, m.cfg.STS)
	if err != nil {
		return nil, nil, err
	}

	enabled, err := regions.EnabledRegions(ctx, bootstrap)
	if err != nil {
		return nil, nil, err
	}

	type regionResult struct {
		region string
		images []models.Image
		err    error
	}
	results := make(chan regionResult, len(enabled))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(m.cfg.Concurrency)
	for _, r := range enabled {
		r := r
		g.Go(func() error {
			c := m.cfg.Factory(r)
			imgs, ierr := images.ImagesInRegion(gctx, c, r)
			results <- regionResult{region: r, images: imgs, err: ierr}
			return nil // never abort the group on a region error
		})
	}
	_ = g.Wait()
	close(results)

	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		AccountID:     acct,
		Action:        m.cfg.Action.Name(),
		DryRun:        m.cfg.DryRun,
	}

	var regionErrs []RegionError
	for r := range results {
		if r.err != nil {
			regionErrs = append(regionErrs, RegionError{Region: r.region, Err: r.err})
			continue
		}
		for _, img := range r.images {
			plan.Entries = append(plan.Entries, planfile.PlanEntry{
				Region:       img.Region,
				ImageID:      img.ID,
				ImageName:    img.Name,
				CreationDate: img.CreationDate,
				SnapshotIDs:  snapshotIDs(img),
				Status:       models.StatusPending,
			})
		}
	}

	return plan, regionErrs, nil
}

// ValidatePlan checks an already-loaded plan against the current credentials
// and the manager's configured action / dry-run. Returns a
// non-nil error if any check fails.
func (m *Manager) ValidatePlan(ctx context.Context, plan *planfile.Plan) error {
	acct, err := account.CallerAccountID(ctx, m.cfg.STS)
	if err != nil {
		return err
	}

	if plan.AccountID != acct {
		return fmt.Errorf("plan was generated for account %s but current credentials are for %s", plan.AccountID, acct)
	}

	if plan.Action != m.cfg.Action.Name() {
		return fmt.Errorf("plan action is %q but --action=%q", plan.Action, m.cfg.Action.Name())
	}

	if plan.DryRun && !m.cfg.DryRun {
		return fmt.Errorf("plan was generated with --dry-run=true; refusing to apply with --dry-run=false (regenerate the plan)")
	}

	return nil
}

// Apply executes pending entries from the store. Each entry's outcome is
// persisted via store.Mark immediately. Per-AMI errors are recorded in the
// plan and the returned ActionResult slice but never abort the run.
func (m *Manager) Apply(ctx context.Context, store *planfile.PlanStore) ([]models.ActionResult, error) {
	plan := store.Plan()
	pending := store.Pending()
	if len(pending) == 0 {
		return nil, nil
	}

	// Group pending indices by region so each region goroutine processes its
	// AMIs sequentially against a single client.
	byRegion := map[string][]int{}
	for _, idx := range pending {
		r := plan.Entries[idx].Region
		byRegion[r] = append(byRegion[r], idx)
	}

	resultsMu := sync.Mutex{}
	var results []models.ActionResult

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(m.cfg.Concurrency)
	for region, indices := range byRegion {
		g.Go(func() error {
			c := m.cfg.Factory(region)
			for _, idx := range indices {
				if err := gctx.Err(); err != nil {
					return err
				}

				entry := plan.Entries[idx]
				img := models.Image{
					ID:           entry.ImageID,
					Name:         entry.ImageName,
					Region:       entry.Region,
					CreationDate: entry.CreationDate,
					BlockDevices: blockDevicesFromSnapshotIDs(entry.SnapshotIDs),
				}
				status := models.StatusCompleted

				err := m.cfg.Action.Apply(gctx, c, img)
				if err != nil {
					status = models.StatusFailed
				}

				if mErr := store.Mark(idx, status, err); mErr != nil {
					return fmt.Errorf("persist plan entry %d: %w", idx, mErr)
				}

				res := models.ActionResult{
					ImageID: entry.ImageID,
					Region:  entry.Region,
					Action:  m.cfg.Action.Name(),
					DryRun:  m.cfg.DryRun,
				}

				if err != nil {
					res.Err = err.Error()
				}

				resultsMu.Lock()
				results = append(results, res)
				resultsMu.Unlock()
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return results, err
	}

	return results, nil
}

// RegionError captures a per-region enumeration failure.
type RegionError struct {
	Region string
	Err    error
}

func (r RegionError) Error() string { return fmt.Sprintf("%s: %v", r.Region, r.Err) }
func (r RegionError) Unwrap() error { return r.Err }

func snapshotIDs(img models.Image) []string {
	if len(img.BlockDevices) == 0 {
		return nil
	}

	out := make([]string, 0, len(img.BlockDevices))
	for _, b := range img.BlockDevices {
		if b.SnapshotID != "" {
			out = append(out, b.SnapshotID)
		}
	}

	return out
}

func blockDevicesFromSnapshotIDs(ids []string) []models.BlockDevice {
	if len(ids) == 0 {
		return nil
	}

	out := make([]models.BlockDevice, len(ids))
	for i, id := range ids {
		out[i] = models.BlockDevice{SnapshotID: id}
	}

	return out
}
