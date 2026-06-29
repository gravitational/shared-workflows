package actions

import (
	"testing"
	"time"

	"github.com/shared-workflows/tools/amicleanup/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeprecateAction_Apply_PassesImageIDAndDeprecateAt(t *testing.T) {
	a := NewDeprecateAction()
	m := newMockEC2()

	currentTime := time.Now().UTC()

	err := a.Apply(t.Context(), m, models.Image{ID: "ami-abc", Region: "us-east-1"})
	require.NoError(t, err)

	require.Len(t, m.enableDeprecation, 1)
	in := m.enableDeprecation[0]
	require.NotNil(t, in.ImageId)
	assert.Equal(t, "ami-abc", *in.ImageId)
	require.NotNil(t, in.DeprecateAt)
	assert.WithinRange(t, *in.DeprecateAt, currentTime.Add(9*time.Minute), currentTime.Add(11*time.Minute))
}

func TestDeprecateAction_Apply_WrapsAPIError(t *testing.T) {
	a := NewDeprecateAction()
	m := newMockEC2()
	m.errFor["EnableImageDeprecation"] = errBoom

	err := a.Apply(t.Context(), m, models.Image{ID: "ami-1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errBoom)
}

func TestDeprecateAction_NameAndDescribe(t *testing.T) {
	a := NewDeprecateAction()
	assert.Equal(t, NameDeprecate, a.Name())
	assert.Contains(t, a.Describe(models.Image{ID: "ami-x", Region: "eu-west-1"}), "ami-x")
	assert.Contains(t, a.Describe(models.Image{ID: "ami-x", Region: "eu-west-1"}), "eu-west-1")
}
