// Package regions enumerates the AWS regions the calling account is enabled in.
package regions

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// regionsClient is the slice of EC2API used by this package. Defining it locally
// (instead of importing ec2iface.EC2API) keeps the package decoupled and makes
// the test fake trivial.
type regionsClient interface {
	DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
}

// EnabledRegions returns the names of regions the calling account can access.
// "Enabled" means OptInStatus is either "opt-in-not-required" (default regions)
// or "opted-in" (manually-enabled regions like ap-southeast-3). Regions with
// status "not-opted-in" are filtered out — calls into them would fail anyway.
func EnabledRegions(ctx context.Context, c regionsClient) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out, err := c.DescribeRegions(ctx, &ec2.DescribeRegionsInput{
		AllRegions: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("describe regions: %w", err)
	}

	var enabled []string
	for _, r := range out.Regions {
		if r.RegionName == nil {
			continue
		}
		if r.OptInStatus == nil {
			continue
		}
		switch *r.OptInStatus {
		case "opt-in-not-required", "opted-in":
			enabled = append(enabled, *r.RegionName)
		}
	}

	return enabled, nil
}
