package client

import (
	"bytes"
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingMock counts every call so readonly_test can assert that the inner
// client is bypassed for write methods and consulted for read methods.
type recordingMock struct {
	calls map[string]int
}

func newRecordingMock() *recordingMock { return &recordingMock{calls: map[string]int{}} }

func (m *recordingMock) DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	m.calls["DescribeRegions"]++
	return &ec2.DescribeRegionsOutput{Regions: []types.Region{{RegionName: ptr.String("us-east-1"), OptInStatus: ptr.String("opt-in-not-required")}}}, nil
}
func (m *recordingMock) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	m.calls["DescribeImages"]++
	return &ec2.DescribeImagesOutput{}, nil
}
func (m *recordingMock) EnableImageDeprecation(ctx context.Context, in *ec2.EnableImageDeprecationInput, _ ...func(*ec2.Options)) (*ec2.EnableImageDeprecationOutput, error) {
	m.calls["EnableImageDeprecation"]++
	return &ec2.EnableImageDeprecationOutput{}, nil
}
func (m *recordingMock) ModifyImageAttribute(ctx context.Context, in *ec2.ModifyImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error) {
	m.calls["ModifyImageAttribute"]++
	return &ec2.ModifyImageAttributeOutput{}, nil
}
func (m *recordingMock) DeregisterImage(ctx context.Context, in *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.calls["DeregisterImage"]++
	return &ec2.DeregisterImageOutput{}, nil
}
func (m *recordingMock) DeleteSnapshot(ctx context.Context, in *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.calls["DeleteSnapshot"]++
	return &ec2.DeleteSnapshotOutput{}, nil
}

func TestReadOnly_ReadsPassThrough(t *testing.T) {
	inner := newRecordingMock()
	logBuf := &bytes.Buffer{}
	r := NewReadOnly(inner, "us-east-1", logBuf)

	_, err := r.DescribeRegions(t.Context(), &ec2.DescribeRegionsInput{})
	require.NoError(t, err)
	_, err = r.DescribeImages(t.Context(), &ec2.DescribeImagesInput{})
	require.NoError(t, err)

	assert.Equal(t, 1, inner.calls["DescribeRegions"])
	assert.Equal(t, 1, inner.calls["DescribeImages"])
	assert.Empty(t, logBuf.String(), "reads must not log")
}

func TestReadOnly_WritesAreNoOps(t *testing.T) {
	tests := []struct {
		name          string
		invoke        func(r *ReadOnly) error
		wantInLog     string
		methodOnInner string
	}{
		{
			name: "EnableImageDeprecation",
			invoke: func(r *ReadOnly) error {
				_, err := r.EnableImageDeprecation(t.Context(), &ec2.EnableImageDeprecationInput{ImageId: ptr.String("ami-1")})
				return err
			},
			wantInLog:     "ami-1",
			methodOnInner: "EnableImageDeprecation",
		},
		{
			name: "ModifyImageAttribute",
			invoke: func(r *ReadOnly) error {
				_, err := r.ModifyImageAttribute(t.Context(), &ec2.ModifyImageAttributeInput{ImageId: ptr.String("ami-2")})
				return err
			},
			wantInLog:     "ami-2",
			methodOnInner: "ModifyImageAttribute",
		},
		{
			name: "DeregisterImage",
			invoke: func(r *ReadOnly) error {
				_, err := r.DeregisterImage(t.Context(), &ec2.DeregisterImageInput{ImageId: ptr.String("ami-3")})
				return err
			},
			wantInLog:     "ami-3",
			methodOnInner: "DeregisterImage",
		},
		{
			name: "DeleteSnapshot",
			invoke: func(r *ReadOnly) error {
				_, err := r.DeleteSnapshot(t.Context(), &ec2.DeleteSnapshotInput{SnapshotId: ptr.String("snap-9")})
				return err
			},
			wantInLog:     "snap-9",
			methodOnInner: "DeleteSnapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := newRecordingMock()
			logBuf := &bytes.Buffer{}
			r := NewReadOnly(inner, "us-east-1", logBuf)
			require.NoError(t, tt.invoke(r))
			assert.Equal(t, 0, inner.calls[tt.methodOnInner], "inner %s must not be invoked", tt.methodOnInner)
			assert.Contains(t, logBuf.String(), tt.wantInLog)
			assert.Contains(t, logBuf.String(), "us-east-1")
			assert.Contains(t, logBuf.String(), "would")
		})
	}
}

func TestReadOnly_NilImageID_StillLogs(t *testing.T) {
	inner := newRecordingMock()
	logBuf := &bytes.Buffer{}
	r := NewReadOnly(inner, "us-east-1", logBuf)

	_, err := r.EnableImageDeprecation(t.Context(), &ec2.EnableImageDeprecationInput{})
	require.NoError(t, err)
	assert.Contains(t, logBuf.String(), "us-east-1")
}
