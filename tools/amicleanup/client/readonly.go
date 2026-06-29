// Package client wraps an EC2 API with a read-only decorator that turns every
// write call into a no-op + log line. Applying this decorator at the factory
// level is how amicleanup enforces dry-run: action implementations always
// invoke writes unconditionally; the decorator absorbs them.
package client

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
)

// ReadOnly wraps an ec2iface.EC2API. Read methods (DescribeRegions, DescribeImages)
// pass through to inner; write methods return a synthetic empty success and
// emit a "would …" line to the supplied io.Writer (typically os.Stderr).
type ReadOnly struct {
	inner  ec2iface.EC2API
	region string
	log    io.Writer
}

// NewReadOnly constructs a dry-run decorator. The region label is included in
// every log line so cross-region runs produce clear output.
func NewReadOnly(inner ec2iface.EC2API, region string, log io.Writer) *ReadOnly {
	return &ReadOnly{inner: inner, region: region, log: log}
}

// DescribeRegions passes through.
func (r *ReadOnly) DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	return r.inner.DescribeRegions(ctx, in, optFns...)
}

// DescribeImages passes through.
func (r *ReadOnly) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return r.inner.DescribeImages(ctx, in, optFns...)
}

// EnableImageDeprecation no-ops; logs `would deprecate <imageID> in <region>`.
func (r *ReadOnly) EnableImageDeprecation(ctx context.Context, in *ec2.EnableImageDeprecationInput, _ ...func(*ec2.Options)) (*ec2.EnableImageDeprecationOutput, error) {
	r.logf("would deprecate %s in %s", deref(in.ImageId), r.region)
	return &ec2.EnableImageDeprecationOutput{}, nil
}

// ModifyImageAttribute no-ops; logs `would modify-image-attribute <imageID> in <region>`.
func (r *ReadOnly) ModifyImageAttribute(ctx context.Context, in *ec2.ModifyImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error) {
	r.logf("would modify-image-attribute %s in %s", deref(in.ImageId), r.region)
	return &ec2.ModifyImageAttributeOutput{}, nil
}

// DeregisterImage no-ops; logs `would deregister <imageID> in <region>`.
func (r *ReadOnly) DeregisterImage(ctx context.Context, in *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	r.logf("would deregister %s in %s", deref(in.ImageId), r.region)
	return &ec2.DeregisterImageOutput{}, nil
}

// DeleteSnapshot no-ops; logs `would delete-snapshot <snapshotID> in <region>`.
func (r *ReadOnly) DeleteSnapshot(ctx context.Context, in *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	r.logf("would delete-snapshot %s in %s", deref(in.SnapshotId), r.region)
	return &ec2.DeleteSnapshotOutput{}, nil
}

func (r *ReadOnly) logf(format string, args ...any) {
	fmt.Fprintln(r.log, fmt.Sprintf(format, args...))
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return *s
}
