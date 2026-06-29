package images

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeImagesClient struct {
	pages     []ec2.DescribeImagesOutput
	pageIndex int
	err       error
	gotInputs []*ec2.DescribeImagesInput
}

func (f *fakeImagesClient) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	f.gotInputs = append(f.gotInputs, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.pageIndex >= len(f.pages) {
		return &ec2.DescribeImagesOutput{}, nil
	}
	page := f.pages[f.pageIndex]
	f.pageIndex++
	return &page, nil
}

func TestImagesInRegion_PaginatesAndAggregates(t *testing.T) {
	c := &fakeImagesClient{
		pages: []ec2.DescribeImagesOutput{
			{
				Images: []types.Image{
					{ImageId: ptr.String("ami-1"), Name: ptr.String("a"), CreationDate: ptr.String("2026-01-01T00:00:00.000Z"), Public: ptr.Bool(false)},
					{ImageId: ptr.String("ami-2"), Name: ptr.String("b"), CreationDate: ptr.String("2026-02-01T00:00:00.000Z"), Public: ptr.Bool(true)},
				},
				NextToken: ptr.String("page-2"),
			},
			{
				Images: []types.Image{
					{ImageId: ptr.String("ami-3"), Name: ptr.String("c"), CreationDate: ptr.String("2026-03-01T00:00:00.000Z"), Public: ptr.Bool(false)},
					{ImageId: ptr.String("ami-4"), Name: ptr.String("d"), CreationDate: ptr.String("2026-04-01T00:00:00.000Z"), Public: ptr.Bool(false)},
				},
				NextToken: ptr.String("page-3"),
			},
			{
				Images: []types.Image{
					{ImageId: ptr.String("ami-5"), Name: ptr.String("e"), CreationDate: ptr.String("2026-04-15T00:00:00.000Z"), Public: ptr.Bool(false)},
				},
			},
		},
	}

	got, err := ImagesInRegion(t.Context(), c, "us-east-1")
	require.NoError(t, err)
	require.Len(t, got, 5)

	ids := make([]string, len(got))
	for i, img := range got {
		ids[i] = img.ID
		assert.Equal(t, "us-east-1", img.Region)
	}
	assert.Equal(t, []string{"ami-1", "ami-2", "ami-3", "ami-4", "ami-5"}, ids)
	assert.True(t, got[1].Public)
	assert.False(t, got[0].Public)

	require.Len(t, c.gotInputs, 3)
	for i, in := range c.gotInputs {
		require.NotNil(t, in.Owners, "page %d", i)
		assert.True(t, slices.Contains(in.Owners, "self"), "page %d Owners=%v", i, in.Owners)
	}
	assert.Nil(t, c.gotInputs[0].NextToken)
	require.NotNil(t, c.gotInputs[1].NextToken)
	assert.Equal(t, "page-2", *c.gotInputs[1].NextToken)
	require.NotNil(t, c.gotInputs[2].NextToken)
	assert.Equal(t, "page-3", *c.gotInputs[2].NextToken)
}

func TestImagesInRegion_ConvertsBlockDeviceMappings(t *testing.T) {
	c := &fakeImagesClient{
		pages: []ec2.DescribeImagesOutput{{
			Images: []types.Image{{
				ImageId:      ptr.String("ami-1"),
				Name:         ptr.String("with-snaps"),
				CreationDate: ptr.String("2026-04-01T00:00:00.000Z"),
				Public:       ptr.Bool(false),
				BlockDeviceMappings: []types.BlockDeviceMapping{
					{
						DeviceName: ptr.String("/dev/xvda"),
						Ebs:        &types.EbsBlockDevice{SnapshotId: ptr.String("snap-1")},
					},
					{
						DeviceName: ptr.String("/dev/xvdb"),
						Ebs:        &types.EbsBlockDevice{SnapshotId: ptr.String("snap-2")},
					},
					// Ephemeral / no-Ebs mapping should be skipped.
					{DeviceName: ptr.String("/dev/xvdc")},
					// Ebs without snapshot id should be skipped.
					{DeviceName: ptr.String("/dev/xvdd"), Ebs: &types.EbsBlockDevice{}},
				},
			}},
		}},
	}

	got, err := ImagesInRegion(t.Context(), c, "us-east-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].BlockDevices, 2)
	assert.Equal(t, "/dev/xvda", got[0].BlockDevices[0].DeviceName)
	assert.Equal(t, "snap-1", got[0].BlockDevices[0].SnapshotID)
	assert.Equal(t, "/dev/xvdb", got[0].BlockDevices[1].DeviceName)
	assert.Equal(t, "snap-2", got[0].BlockDevices[1].SnapshotID)
}

func TestImagesInRegion_ParsesCreationDate(t *testing.T) {
	c := &fakeImagesClient{
		pages: []ec2.DescribeImagesOutput{{
			Images: []types.Image{
				{ImageId: ptr.String("ami-1"), CreationDate: ptr.String("2026-04-01T12:34:56.000Z")},
				// Unparseable date should not abort enumeration; CreationDate is left zero.
				{ImageId: ptr.String("ami-2"), CreationDate: ptr.String("not-a-date")},
				// Nil CreationDate is fine; CreationDate is left zero.
				{ImageId: ptr.String("ami-3")},
			},
		}},
	}

	got, err := ImagesInRegion(t.Context(), c, "us-east-1")
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.False(t, got[0].CreationDate.IsZero())
	assert.Equal(t, 2026, got[0].CreationDate.Year())
	assert.True(t, got[1].CreationDate.IsZero())
	assert.True(t, got[2].CreationDate.IsZero())
}

func TestImagesInRegion_APIError(t *testing.T) {
	want := errors.New("describe-images boom")
	c := &fakeImagesClient{err: want}
	_, err := ImagesInRegion(t.Context(), c, "us-east-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestImagesInRegion_ContextCancelledMidPagination(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := &fakeImagesClient{
		pages: []ec2.DescribeImagesOutput{
			{
				Images:    []types.Image{{ImageId: ptr.String("ami-1")}},
				NextToken: ptr.String("page-2"),
			},
			{
				Images: []types.Image{{ImageId: ptr.String("ami-2")}},
			},
		},
	}

	// Cancel before the first call so the function bails before any IO.
	cancel()
	_, err := ImagesInRegion(ctx, c, "us-east-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
