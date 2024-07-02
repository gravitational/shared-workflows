package cleanup

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/account"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// region Account API
type MockAccountAPI struct {
	MockListRegions func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error)
}

func (maa *MockAccountAPI) ListRegions(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
	return runMock(maa.MockListRegions, ctx, params, optFns)
}

// endregion

// region EC2 API
type MockEC2API struct {
	MockDescribeImages  func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	MockDeregisterImage func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	MockDeleteSnapshot  func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

func (mea *MockEC2API) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return runMock(mea.MockDescribeImages, ctx, params, optFns)
}

func (mea *MockEC2API) DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	return runMock(mea.MockDeregisterImage, ctx, params, optFns)
}

func (mea *MockEC2API) DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	return runMock(mea.MockDeleteSnapshot, ctx, params, optFns)
}

// endregion

// Do common error checking for every mock and then run it
func runMock[TParameters, TInvocationOptions, TResult any](mock func(context.Context, TParameters, ...TInvocationOptions) (TResult, error),
	ctx context.Context, params TParameters, optFns []TInvocationOptions) (TResult, error) {
	if mock == nil {
		panic("Mock API function was called but not implemented")
	}
	return mock(ctx, params, optFns...)
}
