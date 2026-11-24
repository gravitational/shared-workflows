package manager

import (
	"bytes"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shared-workflows/tools/amicleanup/actions"
	"github.com/shared-workflows/tools/amicleanup/client"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/shared-workflows/tools/amicleanup/planfile"
)

func newPerRegion(regionNames []string) map[string]*regionMock {
	per := make(map[string]*regionMock, len(regionNames))
	for _, r := range regionNames {
		per[r] = &regionMock{region: r}
	}
	// Configure the bootstrap region's DescribeRegions response to expose every region.
	if _, ok := per[bootstrapRegion]; !ok {
		per[bootstrapRegion] = &regionMock{region: bootstrapRegion}
	}
	per[bootstrapRegion].describeRegionsOut = regionNames
	return per
}

func TestEnumerate_AggregatesAcrossRegions(t *testing.T) {
	per := newPerRegion([]string{"us-east-1", "eu-west-1", "ap-southeast-2"})
	per["us-east-1"].describeImagesOut = &ec2.DescribeImagesOutput{Images: []types.Image{
		{ImageId: ptr.String("ami-1"), Name: ptr.String("img-1"), CreationDate: ptr.String("2026-04-01T00:00:00.000Z")},
	}}
	per["eu-west-1"].describeImagesErr = errBoom
	per["ap-southeast-2"].describeImagesOut = &ec2.DescribeImagesOutput{Images: []types.Image{
		{ImageId: ptr.String("ami-2"), Name: ptr.String("img-2"), CreationDate: ptr.String("2026-04-02T00:00:00.000Z")},
		{ImageId: ptr.String("ami-3"), Name: ptr.String("img-3"), CreationDate: ptr.String("2026-04-03T00:00:00.000Z")},
	}}

	m := New(Config{
		Factory: fakeFactory(per),
		STS:     &stsMock{account: "123456789012"},
		Action:  actions.NewDeprecateAction(),
		DryRun:  true,
	})

	plan, regionErrs, err := m.Enumerate(t.Context())
	require.NoError(t, err)
	require.Len(t, regionErrs, 1)
	assert.Equal(t, "eu-west-1", regionErrs[0].Region)
	assert.ErrorIs(t, regionErrs[0].Err, errBoom)

	require.Len(t, plan.Entries, 3)
	gotIDs := []string{plan.Entries[0].ImageID, plan.Entries[1].ImageID, plan.Entries[2].ImageID}
	assert.ElementsMatch(t, []string{"ami-1", "ami-2", "ami-3"}, gotIDs)
	assert.Equal(t, "deprecate", plan.Action)
	assert.Equal(t, "123456789012", plan.AccountID)
	assert.True(t, plan.DryRun)
	assert.Equal(t, planfile.SchemaVersion, plan.SchemaVersion)
}

func TestEnumerate_RespectsConcurrencyLimit(t *testing.T) {
	const N = 16
	regionNames := make([]string, N)
	for i := 0; i < N; i++ {
		regionNames[i] = fmtRegion(i)
	}
	per := newPerRegion(regionNames)

	var inFlight, maxInFlight int64
	for _, mock := range per {
		mock.describeImagesOut = &ec2.DescribeImagesOutput{}
		mock.describeDelay = 20 * time.Millisecond
		mock.inFlight = &inFlight
		mock.maxInFlight = &maxInFlight
	}

	m := New(Config{
		Factory:     fakeFactory(per),
		STS:         &stsMock{account: "123456789012"},
		Action:      actions.NewDeprecateAction(),
		Concurrency: 2,
	})

	_, _, err := m.Enumerate(t.Context())
	require.NoError(t, err)
	assert.LessOrEqual(t, atomic.LoadInt64(&maxInFlight), int64(2), "max in-flight DescribeImages exceeded SetLimit")
}

func TestApply_DispatchesAction_Deprecate(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		AccountID:     "123",
		Action:        actions.NameDeprecate,
		Entries: []planfile.PlanEntry{
			{Region: "us-east-1", ImageID: "ami-a", Status: models.StatusPending},
			{Region: "eu-west-1", ImageID: "ami-b", Status: models.StatusPending},
		},
	}
	store := mustNewStore(t, plan)

	per := newPerRegion([]string{"us-east-1", "eu-west-1"})
	m := New(Config{
		Factory: fakeFactory(per),
		STS:     &stsMock{account: "123"},
		Action:  actions.NewDeprecateAction(),
	})

	results, err := m.Apply(t.Context(), store)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, 1, len(per["us-east-1"].enableDeprecation))
	assert.Equal(t, 1, len(per["eu-west-1"].enableDeprecation))
	assert.Empty(t, per["us-east-1"].modifyAttribute)
	assert.Empty(t, per["us-east-1"].deregisterImage)
}

func TestApply_DispatchesAction_MakePublic(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		Action:        actions.NameMakePublic,
		Entries: []planfile.PlanEntry{
			{Region: "us-east-1", ImageID: "ami-a", Status: models.StatusPending},
		},
	}
	store := mustNewStore(t, plan)

	per := newPerRegion([]string{"us-east-1"})
	m := New(Config{
		Factory: fakeFactory(per),
		STS:     &stsMock{account: "123"},
		Action:  actions.NewMakePublicAction(),
	})

	_, err := m.Apply(t.Context(), store)
	require.NoError(t, err)
	require.Len(t, per["us-east-1"].modifyAttribute, 1)
	assert.Empty(t, per["us-east-1"].enableDeprecation)
	assert.Empty(t, per["us-east-1"].deregisterImage)
}

func TestApply_DispatchesAction_DeleteWithSnapshots(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		Action:        actions.NameDelete,
		Entries: []planfile.PlanEntry{
			{Region: "us-east-1", ImageID: "ami-a", SnapshotIDs: []string{"snap-1", "snap-2"}, Status: models.StatusPending},
		},
	}
	store := mustNewStore(t, plan)

	per := newPerRegion([]string{"us-east-1"})
	m := New(Config{
		Factory: fakeFactory(per),
		STS:     &stsMock{account: "123"},
		Action:  actions.NewDeleteAction(),
	})

	_, err := m.Apply(t.Context(), store)
	require.NoError(t, err)
	assert.Len(t, per["us-east-1"].deregisterImage, 1)
	assert.Len(t, per["us-east-1"].deleteSnapshot, 2)
}

func TestApply_DryRun_NoWritesHitInnerMock(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		Action:        actions.NameDeprecate,
		Entries: []planfile.PlanEntry{
			{Region: "us-east-1", ImageID: "ami-a", Status: models.StatusPending},
		},
		DryRun: true,
	}
	store := mustNewStore(t, plan)

	per := newPerRegion([]string{"us-east-1"})
	logBuf := &bytes.Buffer{}
	wrappedFactory := func(region string) ec2iface.EC2API {
		return client.NewReadOnly(per[region], region, logBuf)
	}

	m := New(Config{
		Factory: wrappedFactory,
		STS:     &stsMock{account: "123"},
		Action:  actions.NewDeprecateAction(),
		DryRun:  true,
	})

	_, err := m.Apply(t.Context(), store)
	require.NoError(t, err)
	assert.Empty(t, per["us-east-1"].enableDeprecation, "writes must not reach inner mock in dry-run")
	assert.Contains(t, logBuf.String(), "would deprecate")
	assert.Contains(t, logBuf.String(), "ami-a")
}

func TestApply_ResumeSkipsCompletedEntries(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		Action:        actions.NameDeprecate,
		Entries: []planfile.PlanEntry{
			{Region: "us-east-1", ImageID: "ami-1", Status: models.StatusCompleted},
			{Region: "us-east-1", ImageID: "ami-2", Status: models.StatusPending},
			{Region: "us-east-1", ImageID: "ami-3", Status: models.StatusFailed},
			{Region: "us-east-1", ImageID: "ami-4", Status: models.StatusCompleted},
			{Region: "us-east-1", ImageID: "ami-5", Status: models.StatusPending},
		},
	}
	store := mustNewStore(t, plan)

	per := newPerRegion([]string{"us-east-1"})
	m := New(Config{
		Factory: fakeFactory(per),
		STS:     &stsMock{account: "123"},
		Action:  actions.NewDeprecateAction(),
	})

	_, err := m.Apply(t.Context(), store)
	require.NoError(t, err)

	// 3 calls (the pending and failed entries), not 5.
	require.Len(t, per["us-east-1"].enableDeprecation, 3)
	gotIDs := []string{}
	for _, in := range per["us-east-1"].enableDeprecation {
		gotIDs = append(gotIDs, *in.ImageId)
	}
	assert.ElementsMatch(t, []string{"ami-2", "ami-3", "ami-5"}, gotIDs)

	finalPlan := store.Plan()
	for i, e := range finalPlan.Entries {
		assert.Equal(t, models.StatusCompleted, e.Status, "entry %d should be completed", i)
	}
}

func TestValidatePlan_AccountMismatch(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		AccountID:     "111",
		Action:        actions.NameDeprecate,
	}
	m := New(Config{
		STS:    &stsMock{account: "999"},
		Action: actions.NewDeprecateAction(),
	})
	err := m.ValidatePlan(t.Context(), plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "111")
	assert.Contains(t, err.Error(), "999")
}

func TestValidatePlan_ActionMismatch(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		AccountID:     "123",
		Action:        actions.NameDelete,
	}
	m := New(Config{
		STS:    &stsMock{account: "123"},
		Action: actions.NewMakePublicAction(),
	})
	err := m.ValidatePlan(t.Context(), plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), actions.NameDelete)
	assert.Contains(t, err.Error(), actions.NameMakePublic)
}

func TestValidatePlan_DryRunMismatch(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		AccountID:     "123",
		Action:        actions.NameDeprecate,
		DryRun:        true,
	}
	m := New(Config{
		STS:    &stsMock{account: "123"},
		Action: actions.NewDeprecateAction(),
		DryRun: false,
	})
	err := m.ValidatePlan(t.Context(), plan)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dry-run")
}

func TestValidatePlan_AllMatch(t *testing.T) {
	plan := &planfile.Plan{
		SchemaVersion: planfile.SchemaVersion,
		AccountID:     "123",
		Action:        actions.NameDeprecate,
		DryRun:        false,
	}
	m := New(Config{
		STS:    &stsMock{account: "123"},
		Action: actions.NewDeprecateAction(),
		DryRun: false,
	})
	require.NoError(t, m.ValidatePlan(t.Context(), plan))
}

func mustNewStore(t *testing.T, plan *planfile.Plan) *planfile.PlanStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	store, err := planfile.NewStore(path, plan)
	require.NoError(t, err)
	return store
}

// fmtRegion returns deterministic region names for the concurrency test. Index
// 0 is the well-known bootstrapRegion so DescribeRegions has a predictable home.
func fmtRegion(i int) string {
	if i == 0 {
		return bootstrapRegion
	}
	return "test-region-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
