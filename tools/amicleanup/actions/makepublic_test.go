package actions

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakePublicAction_Apply_AddsAllGroup(t *testing.T) {
	a := NewMakePublicAction()
	m := newMockEC2()
	require.NoError(t, a.Apply(t.Context(), m, models.Image{ID: "ami-abc"}))

	require.Len(t, m.modifyAttribute, 1)
	in := m.modifyAttribute[0]
	require.NotNil(t, in.ImageId)
	assert.Equal(t, "ami-abc", *in.ImageId)
	require.NotNil(t, in.LaunchPermission)
	require.Len(t, in.LaunchPermission.Add, 1)
	assert.Equal(t, types.PermissionGroupAll, in.LaunchPermission.Add[0].Group)
	assert.Empty(t, in.LaunchPermission.Remove, "make-public must not Remove")
}

func TestMakePublicAction_Apply_WrapsAPIError(t *testing.T) {
	a := NewMakePublicAction()
	m := newMockEC2()
	m.errFor["ModifyImageAttribute"] = errBoom

	err := a.Apply(t.Context(), m, models.Image{ID: "ami-1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

func TestMakePublicAction_NameAndDescribe(t *testing.T) {
	a := NewMakePublicAction()
	assert.Equal(t, NameMakePublic, a.Name())
	assert.Contains(t, a.Describe(models.Image{ID: "ami-x", Region: "us-east-1"}), "ami-x")
}
