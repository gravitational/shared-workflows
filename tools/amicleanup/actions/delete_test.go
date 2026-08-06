package actions

import (
	"testing"

	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteAction_Apply_DeregistersAndDeletesAllSnapshots(t *testing.T) {
	a := NewDeleteAction()
	m := newMockEC2()

	img := models.Image{
		ID: "ami-abc",
		BlockDevices: []models.BlockDevice{
			{DeviceName: "/dev/xvda", SnapshotID: "snap-1"},
			{DeviceName: "/dev/xvdb", SnapshotID: "snap-2"},
		},
	}

	require.NoError(t, a.Apply(t.Context(), m, img))

	require.Len(t, m.deregisterImage, 1)
	require.NotNil(t, m.deregisterImage[0].ImageId)
	assert.Equal(t, "ami-abc", *m.deregisterImage[0].ImageId)

	require.Len(t, m.deleteSnapshot, 2)
	require.NotNil(t, m.deleteSnapshot[0].SnapshotId)
	assert.Equal(t, "snap-1", *m.deleteSnapshot[0].SnapshotId)
	require.NotNil(t, m.deleteSnapshot[1].SnapshotId)
	assert.Equal(t, "snap-2", *m.deleteSnapshot[1].SnapshotId)
}

func TestDeleteAction_Apply_NoSnapshots_OnlyDeregisters(t *testing.T) {
	a := NewDeleteAction()
	m := newMockEC2()

	require.NoError(t, a.Apply(t.Context(), m, models.Image{ID: "ami-1"}))
	require.Len(t, m.deregisterImage, 1)
	assert.Empty(t, m.deleteSnapshot)
}

func TestDeleteAction_Apply_DeregisterError_StopsBeforeSnapshots(t *testing.T) {
	a := NewDeleteAction()
	m := newMockEC2()
	m.errFor["DeregisterImage"] = errBoom

	err := a.Apply(t.Context(), m, models.Image{
		ID:           "ami-1",
		BlockDevices: []models.BlockDevice{{SnapshotID: "snap-1"}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
	assert.Empty(t, m.deleteSnapshot, "snapshot deletes must not run if deregister fails")
}

func TestDeleteAction_Apply_SnapshotError_ReportsAfterDeregister(t *testing.T) {
	a := NewDeleteAction()
	m := newMockEC2()
	m.errFor["DeleteSnapshot"] = errBoom

	img := models.Image{
		ID: "ami-1",
		BlockDevices: []models.BlockDevice{
			{SnapshotID: "snap-1"},
			{SnapshotID: "snap-2"},
		},
	}
	err := a.Apply(t.Context(), m, img)
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
	assert.Contains(t, err.Error(), "ami-1")

	require.Len(t, m.deregisterImage, 1, "deregister must still have happened")
	// Both snapshot deletions should be attempted even if the first failed,
	// so the error message can list every offender.
	assert.Len(t, m.deleteSnapshot, 2)
}

func TestDeleteAction_NameAndDescribe(t *testing.T) {
	a := NewDeleteAction()
	assert.Equal(t, NameDelete, a.Name())
	assert.Contains(t, a.Describe(models.Image{ID: "ami-x", Region: "us-east-1"}), "ami-x")
	assert.Contains(t, a.Describe(models.Image{
		ID:           "ami-x",
		Region:       "us-east-1",
		BlockDevices: []models.BlockDevice{{SnapshotID: "snap-9"}},
	}), "snap-9")
}

func TestNewByName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
		typeOk  func(Action) bool
	}{
		{name: NameDeprecate, typeOk: func(a Action) bool { _, ok := a.(*DeprecateAction); return ok }},
		{name: NameMakePublic, typeOk: func(a Action) bool { _, ok := a.(*MakePublicAction); return ok }},
		{name: NameMakePrivate, typeOk: func(a Action) bool { _, ok := a.(*MakePrivateAction); return ok }},
		{name: NameDelete, typeOk: func(a Action) bool { _, ok := a.(*DeleteAction); return ok }},
		{name: "bogus", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewByName(c.name)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.True(t, c.typeOk(got), "wrong concrete type for %s", c.name)
		})
	}
}
