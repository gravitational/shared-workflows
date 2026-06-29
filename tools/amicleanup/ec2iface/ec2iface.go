// Package ec2iface defines the narrow AWS API surface amicleanup uses.
// Concrete *ec2.Client and *sts.Client values from aws-sdk-go-v2 satisfy these
// interfaces directly; tests substitute hand-rolled fakes.
package ec2iface

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// EC2API is the subset of *ec2.Client amicleanup invokes.
type EC2API interface {
	DescribeRegions(ctx context.Context, params *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	EnableImageDeprecation(ctx context.Context, params *ec2.EnableImageDeprecationInput, optFns ...func(*ec2.Options)) (*ec2.EnableImageDeprecationOutput, error)
	ModifyImageAttribute(ctx context.Context, params *ec2.ModifyImageAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

// STSAPI is the subset of *sts.Client amicleanup invokes.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// RegionalClientFactory returns an EC2API bound to a specific AWS region.
// Production code constructs *ec2.Client values via aws.Config + ec2.NewFromConfig
// with the region option; tests inject fakes keyed by region.
type RegionalClientFactory func(region string) EC2API
