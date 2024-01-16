/*
 * Copyright 2024 Gravitational, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// TODO move this under /libs at some point, however it is nowhere near
// large enough to justify separate module today
package internal

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/account"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
)

const DryRunErrorCode = "DryRunOperation" // found via experimentation, there is no const in the SDK for this

func IsDryRunError(err error) bool {
	if err == nil {
		return false
	}

	// This is awful but it is the only way that I could find to check
	// if the AWS API returned a dry run error
	if operationError, ok := err.(*smithy.OperationError); ok {
		err = operationError.Unwrap()
		if responseError, ok := err.(*http.ResponseError); ok {
			err = responseError.Err
			if genericError, ok := err.(*smithy.GenericAPIError); ok {
				return genericError.Code == DryRunErrorCode
			}
		}
	}

	return false
}

// These are primarily used for mocks while testing, and follows a similar pattern to
// github.com/gravitational/cloud/pkg/aws
// TODO consider writing a `go generate` program for this. This will quickly get out
// of hand if used frequently.

// region Account API
type IAccountApi interface {
	ListRegions(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error)
}

type AccountApi struct {
	cfg *aws.Config
	*account.Client
}

func NewAccountApi(cfg *aws.Config) IAccountApi {
	return &AccountApi{
		cfg:    cfg,
		Client: account.NewFromConfig(*cfg),
	}
}

type MockAccountAPI struct {
	MockListRegions func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error)
}

func (maa *MockAccountAPI) ListRegions(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
	return runMock(maa.MockListRegions, ctx, params, optFns)
}

// endregion

// region EC2 API
type IEc2Api interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

type Ec2Api struct {
	cfg *aws.Config
	*ec2.Client
}

func NewEc2Api(cfg *aws.Config) IEc2Api {
	return &Ec2Api{
		cfg:    cfg,
		Client: ec2.NewFromConfig(*cfg),
	}
}

type MockEc2API struct {
	MockDescribeImages  func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	MockDeregisterImage func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	MockDeleteSnapshot  func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

func (mea *MockEc2API) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return runMock(mea.MockDescribeImages, ctx, params, optFns)
}

func (mea *MockEc2API) DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	return runMock(mea.MockDeregisterImage, ctx, params, optFns)
}

func (mea *MockEc2API) DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
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
