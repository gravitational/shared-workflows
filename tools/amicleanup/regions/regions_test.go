package regions

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRegionsClient is a minimal EC2API stand-in for region-listing tests.
// It only implements DescribeRegions; other methods are unused in this package.
type fakeRegionsClient struct {
	output   *ec2.DescribeRegionsOutput
	err      error
	gotInput *ec2.DescribeRegionsInput
}

func (f *fakeRegionsClient) DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	f.gotInput = in
	if f.err != nil {
		return nil, f.err
	}
	return f.output, nil
}

func TestEnabledRegions_FiltersByOptInStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  []types.Region
		expect []string
	}{
		{
			name: "mixed statuses",
			input: []types.Region{
				{RegionName: ptr.String("us-east-1"), OptInStatus: ptr.String("opt-in-not-required")},
				{RegionName: ptr.String("ap-southeast-3"), OptInStatus: ptr.String("opted-in")},
				{RegionName: ptr.String("me-south-1"), OptInStatus: ptr.String("not-opted-in")},
				{RegionName: ptr.String("eu-west-1"), OptInStatus: ptr.String("opt-in-not-required")},
			},
			expect: []string{"us-east-1", "ap-southeast-3", "eu-west-1"},
		},
		{
			name:   "empty input",
			input:  nil,
			expect: nil,
		},
		{
			name: "all disabled",
			input: []types.Region{
				{RegionName: ptr.String("me-south-1"), OptInStatus: ptr.String("not-opted-in")},
				{RegionName: ptr.String("af-south-1"), OptInStatus: ptr.String("not-opted-in")},
			},
			expect: nil,
		},
		{
			name: "skips entries with nil RegionName",
			input: []types.Region{
				{RegionName: nil, OptInStatus: ptr.String("opt-in-not-required")},
				{RegionName: ptr.String("us-east-1"), OptInStatus: ptr.String("opt-in-not-required")},
			},
			expect: []string{"us-east-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &fakeRegionsClient{output: &ec2.DescribeRegionsOutput{Regions: tt.input}}
			got, err := EnabledRegions(t.Context(), c)
			require.NoError(t, err)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestEnabledRegions_PassesAllRegionsFalse(t *testing.T) {
	c := &fakeRegionsClient{output: &ec2.DescribeRegionsOutput{}}
	_, err := EnabledRegions(t.Context(), c)
	require.NoError(t, err)
	require.NotNil(t, c.gotInput)
	require.NotNil(t, c.gotInput.AllRegions)
	assert.False(t, *c.gotInput.AllRegions, "should query enabled regions only")
}

func TestEnabledRegions_APIError(t *testing.T) {
	want := errors.New("describe-regions boom")
	c := &fakeRegionsClient{err: want}
	_, err := EnabledRegions(t.Context(), c)
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestEnabledRegions_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := &fakeRegionsClient{output: &ec2.DescribeRegionsOutput{}}
	_, err := EnabledRegions(ctx, c)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
