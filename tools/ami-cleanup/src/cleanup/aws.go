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
package cleanup

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
type AccountAPI interface {
	ListRegions(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error)
}

type AWSAccountAPI struct {
	cfg *aws.Config
	*account.Client
}

func NewAccountAPI(cfg *aws.Config) AccountAPI {
	return &AWSAccountAPI{
		cfg:    cfg,
		Client: account.NewFromConfig(*cfg),
	}
}

// endregion

// region EC2 API
type EC2API interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

type AWSEC2API struct {
	cfg *aws.Config
	*ec2.Client
}

func NewEC2API(cfg *aws.Config) EC2API {
	return &AWSEC2API{
		cfg:    cfg,
		Client: ec2.NewFromConfig(*cfg),
	}
}

// endregion
