package actions

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// mockEC2 is a captured-call fake that implements ec2iface.EC2API. It records
// every write call's input so the per-action tests can assert exact request shape.
//
// nextErr lets a single test inject an error from a specific method by setting
// errFor[methodName] before the call. Read-only methods (DescribeRegions /
// DescribeImages) are not exercised by action tests; they panic if called to
// catch any accidental coupling.
type mockEC2 struct {
	enableDeprecation []*ec2.EnableImageDeprecationInput
	modifyAttribute   []*ec2.ModifyImageAttributeInput
	deregisterImage   []*ec2.DeregisterImageInput
	deleteSnapshot    []*ec2.DeleteSnapshotInput

	errFor map[string]error // method name -> error to return on next (and subsequent) calls
}

func newMockEC2() *mockEC2 { return &mockEC2{errFor: map[string]error{}} }

func (m *mockEC2) DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, _ ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	panic("DescribeRegions not expected in action tests")
}

func (m *mockEC2) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	panic("DescribeImages not expected in action tests")
}

func (m *mockEC2) EnableImageDeprecation(ctx context.Context, in *ec2.EnableImageDeprecationInput, _ ...func(*ec2.Options)) (*ec2.EnableImageDeprecationOutput, error) {
	m.enableDeprecation = append(m.enableDeprecation, in)
	if e := m.errFor["EnableImageDeprecation"]; e != nil {
		return nil, e
	}
	return &ec2.EnableImageDeprecationOutput{}, nil
}

func (m *mockEC2) ModifyImageAttribute(ctx context.Context, in *ec2.ModifyImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error) {
	m.modifyAttribute = append(m.modifyAttribute, in)
	if e := m.errFor["ModifyImageAttribute"]; e != nil {
		return nil, e
	}
	return &ec2.ModifyImageAttributeOutput{}, nil
}

func (m *mockEC2) DeregisterImage(ctx context.Context, in *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	m.deregisterImage = append(m.deregisterImage, in)
	if e := m.errFor["DeregisterImage"]; e != nil {
		return nil, e
	}
	return &ec2.DeregisterImageOutput{}, nil
}

func (m *mockEC2) DeleteSnapshot(ctx context.Context, in *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.deleteSnapshot = append(m.deleteSnapshot, in)
	if e := m.errFor["DeleteSnapshot"]; e != nil {
		return nil, e
	}
	return &ec2.DeleteSnapshotOutput{}, nil
}

// errBoom is a sentinel used by error-path tests so they can assert with errors.Is.
var errBoom = errors.New("boom")
