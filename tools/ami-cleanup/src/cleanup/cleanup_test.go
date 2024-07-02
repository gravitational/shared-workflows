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

package cleanup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/account"
	accounttypes "github.com/aws/aws-sdk-go-v2/service/account/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestGetEnabledRegions(t *testing.T) {
	// These are defined as separate variables so that pointers can reference the values
	usEast1 := "us-east-1"
	usEast2 := "us-east-2"
	usWest1 := "us-west-1"
	usWest2 := "us-west-2"
	expectedRegions := []accounttypes.Region{
		{
			RegionName:      &usEast1,
			RegionOptStatus: accounttypes.RegionOptStatusEnabled,
		},
		{
			RegionName:      &usEast2,
			RegionOptStatus: accounttypes.RegionOptStatusEnabledByDefault,
		},
		{
			RegionName:      &usWest1,
			RegionOptStatus: accounttypes.RegionOptStatusEnabledByDefault,
		},
		{
			RegionName:      &usWest2,
			RegionOptStatus: accounttypes.RegionOptStatusEnabled,
		},
	}

	tests := []struct {
		desc            string
		mockListRegions func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error)
		expectedRegions []accounttypes.Region
		checkError      require.ErrorAssertionFunc
	}{
		{
			desc: "fail if API call errors",
			mockListRegions: func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
				return nil, errors.New("some API call error")
			},
			checkError: require.Error,
		},
		{
			desc: "no error when API call does not error",
			mockListRegions: func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
				return &account.ListRegionsOutput{}, nil
			},
		},
		{
			desc: "only enabled regions requested",
			mockListRegions: func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
				enabledStatuses := []accounttypes.RegionOptStatus{accounttypes.RegionOptStatusEnabled, accounttypes.RegionOptStatusEnabledByDefault}
				slices.Sort(enabledStatuses)
				slices.Sort(params.RegionOptStatusContains)
				if slices.Compare(enabledStatuses, params.RegionOptStatusContains) != 0 {
					return nil, trace.Errorf("the requested region statuses %#v did not match the expected region statuses %#v", params.RegionOptStatusContains, enabledStatuses)
				}
				return &account.ListRegionsOutput{}, nil
			},
		},
		{
			desc: "all results are returned",
			mockListRegions: func(ctx context.Context, params *account.ListRegionsInput, optFns ...func(*account.Options)) (*account.ListRegionsOutput, error) {
				return &account.ListRegionsOutput{
					Regions:   expectedRegions,
					NextToken: nil,
				}, nil
			},
			expectedRegions: expectedRegions,
		},
	}

	for _, test := range tests {
		if test.checkError == nil {
			test.checkError = require.NoError
		}

		t.Run(test.desc, func(t *testing.T) {
			// Setup the application instance
			application := &ApplicationInstance{
				accountClientGenerator: func(cfg *aws.Config) AccountAPI {
					return &MockAccountAPI{
						MockListRegions: test.mockListRegions,
					}
				},
			}

			// Run the function under test
			regions, err := application.getEnabledRegions(context.Background(), application.accountClientGenerator(nil))

			// Verify the results
			test.checkError(t, err)
			require.ElementsMatch(t, regions, test.expectedRegions, "the returned regions did not match the expected regions")
		})
	}
}

func TestGetAllWithPagination(t *testing.T) {
	actionSuppliedResults := [][]string{
		{
			"result 1",
			"result 2",
		},
		{
			"result 3",
			"result 4",
		},
	}
	allResults := make([]string, 0)
	for _, actionSuppliedResult := range actionSuppliedResults {
		allResults = append(allResults, actionSuppliedResult...)
	}

	tests := []struct {
		desc            string
		action          func(previousToken *string) (nextToken *string, results []string, err error)
		expectedResults []string
		checkError      require.ErrorAssertionFunc
	}{
		{
			desc: "fail if action errors",
			action: func(previousToken *string) (nextToken *string, results []string, err error) {
				return nil, nil, trace.Errorf("some action error")
			},
			checkError: require.Error,
		},
		{
			desc: "first token is nil",
			action: func(previousToken *string) (nextToken *string, results []string, err error) {
				if previousToken != nil {
					return nil, nil, trace.Errorf("the first token was %q, expected nil", *previousToken)
				}

				return nil, nil, nil
			},
		},
		{
			desc: "all results returned when there is a single page of results",
			action: func(previousToken *string) (nextToken *string, results []string, err error) {
				return nil, actionSuppliedResults[0], nil
			},
			expectedResults: actionSuppliedResults[0],
		},
		{
			desc: "all results returned for multiple pages of results",
			action: func(previousToken *string) (nextToken *string, results []string, err error) {
				// Resolve the token returned by the previous result to an index in the action result array
				pageRegionIndex := 0
				if previousToken != nil {
					var err error
					pageRegionIndex, err = strconv.Atoi(*previousToken)
					if err != nil {
						return nil, nil, trace.Errorf("failed to convert next token %q to integer", *previousToken)
					}
				}

				if pageRegionIndex > len(actionSuppliedResults)-1 {
					return nil, nil, trace.Errorf("requested more responses (page %d) than were available", pageRegionIndex)
				}

				var responseToken *string
				if pageRegionIndex != len(actionSuppliedResults)-1 {
					// When not the last page, return the next page as the response token
					nextPageToken := strconv.Itoa(pageRegionIndex + 1)
					responseToken = &nextPageToken
				}

				return responseToken, actionSuppliedResults[pageRegionIndex], nil
			},
			expectedResults: allResults,
		},
	}

	for _, test := range tests {
		if test.checkError == nil {
			test.checkError = require.NoError
		}

		t.Run(test.desc, func(t *testing.T) {
			results, err := getAllWithPagination(test.action)
			test.checkError(t, err)
			require.ElementsMatch(t, results, test.expectedResults, "the returned results did not match the expected results")
		})
	}
}

func TestGetDevImagesInRegion(t *testing.T) {
	tests := []struct {
		desc               string
		mockDescribeImages func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
		expectedImages     []ec2types.Image
		doDryRun           bool
		checkError         require.ErrorAssertionFunc
	}{
		{
			desc: "fail if API call errors",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return nil, trace.Errorf("some API call error")
			},
			checkError: require.Error,
		},
		{
			desc: "no error when API call does not error",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "request only available images",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				foundMatchingFilter := false
				for _, filter := range params.Filters {
					if *filter.Name != "state" {
						continue
					}

					if foundMatchingFilter {
						return nil, trace.Errorf("found multiple image state filters")
					}

					stateFilterCount := len(filter.Values)
					if stateFilterCount != 1 {
						return nil, trace.Errorf("expected one image state filter, found %d", stateFilterCount)
					}

					if filter.Values[0] != "available" {
						return nil, trace.Errorf("image state filter found, but was set to %q", filter.Values[0])
					}

					foundMatchingFilter = true
				}

				if !foundMatchingFilter {
					return nil, trace.Errorf("did not find any image state filters in request")
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "request only dev images",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				foundMatchingFilter := false
				for _, filter := range params.Filters {
					if *filter.Name != "name" {
						continue
					}

					if foundMatchingFilter {
						return nil, trace.Errorf("found multiple image name filters")
					}

					nameFilterCount := len(filter.Values)
					if nameFilterCount != 1 {
						return nil, trace.Errorf("expected one image state name, found %d", nameFilterCount)
					}

					nameFilter := filter.Values[0]
					if !strings.Contains(nameFilter, "dev") {
						// This is not strictly true but should cover the majority of cases.
						// Filtering details are available at
						// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Filtering.html#Filtering_Resources_CLI
						// At the time of writing it is not worth the development effort required for building a filter
						// parser to validate this further.
						return nil, trace.Errorf("the name filter %q is not limited to dev images", nameFilter)
					}

					foundMatchingFilter = true
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "requests deprecated images",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				if !*params.IncludeDeprecated {
					return nil, trace.Errorf("API call did not request deprecated images")
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "requests disabled images",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				if !*params.IncludeDisabled {
					return nil, trace.Errorf("API call did not request disabled images")
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "only requests self-owned images",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				ownerCount := len(params.Owners)
				if ownerCount != 1 {
					return nil, trace.Errorf("expected one image owner in the API call, got %d", ownerCount)
				}

				if params.Owners[0] != "self" {
					return nil, trace.Errorf("requested images owned by %q instead of self", params.Owners[0])
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
		},
		{
			desc: "requests not as a dry run even when set in the application",
			mockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				if params.DryRun != nil && *params.DryRun {
					return nil, trace.Errorf("API call was set to a do dry run")
				}

				return &ec2.DescribeImagesOutput{}, nil
			},
			doDryRun: true,
		},
	}

	for _, test := range tests {
		if test.checkError == nil {
			test.checkError = require.NoError
		}

		t.Run(test.desc, func(t *testing.T) {
			// Setup the application instance
			application := &ApplicationInstance{
				shouldDoDryRun: test.doDryRun,
				ec2ClientGenerator: func(cfg *aws.Config) EC2API {
					return &MockEC2API{
						MockDescribeImages: test.mockDescribeImages,
					}
				},
			}

			// Run the function under test
			images, err := application.getDevImagesInRegion(context.Background(), application.ec2ClientGenerator(nil))

			// Verify the results
			test.checkError(t, err)
			require.ElementsMatch(t, images, test.expectedImages, "the returned images did not match the expected images")
		})
	}
}

func TestDeleteSnapshotsForImage(t *testing.T) {
	singleSnapshotImage := generateImageFixture("single snapshot image", "", 1)
	multipleSnapshotImage := generateImageFixture("multiple snapshot image", "", 3)

	tests := []struct {
		desc                   string
		image                  imageFixture
		mockDeleteSnapshot     func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
		doDryRun               bool
		checkError             require.ErrorAssertionFunc
		shouldIgnoreSpaceCheck bool
	}{
		{
			desc:  "fail if API call errors",
			image: singleSnapshotImage,
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				return nil, trace.Errorf("some API call error")
			},
			checkError:             require.Error,
			shouldIgnoreSpaceCheck: true,
		},
		{
			desc: "no error when API call does not error",
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				return &ec2.DeleteSnapshotOutput{}, nil
			},
		},
		{
			desc:  "correct recovered space reported when single snapshot",
			image: singleSnapshotImage,
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				return &ec2.DeleteSnapshotOutput{}, nil
			},
		},
		// This also checks to ensure that all snapshots were deleted
		{
			desc:  "correct recovered space reported when multiple snapshot",
			image: multipleSnapshotImage,
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				return &ec2.DeleteSnapshotOutput{}, nil
			},
		},
		{
			desc: "requests as a dry run when set in the application",
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				if !*params.DryRun {
					return nil, trace.Errorf("API call was not set to a do dry run")
				}

				return &ec2.DeleteSnapshotOutput{}, nil
			},
			doDryRun: true,
		},
		{
			desc: "requests as not a dry run when not set in the application",
			mockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
				if params.DryRun != nil && *params.DryRun {
					return nil, trace.Errorf("API call was set to a do dry run")
				}

				return &ec2.DeleteSnapshotOutput{}, nil
			},
			doDryRun: false,
		},
	}

	for _, test := range tests {
		if test.checkError == nil {
			test.checkError = require.NoError
		}

		t.Run(test.desc, func(t *testing.T) {
			// Setup the application instance
			application := &ApplicationInstance{
				shouldDoDryRun: test.doDryRun,
				ec2ClientGenerator: func(cfg *aws.Config) EC2API {
					return &MockEC2API{
						MockDeleteSnapshot: test.mockDeleteSnapshot,
					}
				},
			}

			// Run the function under test
			recoveredSpace, err := application.deleteSnapshotsForImage(context.Background(), application.ec2ClientGenerator(nil), test.image.Image)

			// Verify the results
			test.checkError(t, err)
			if !test.shouldIgnoreSpaceCheck {
				require.Equal(t, test.image.totalSize, recoveredSpace, "the recovered space did not match the expected recovered space")
			}
		})
	}
}

func TestCleanupImageIfOld(t *testing.T) {
	oldImage := generateImageFixture("old image", "2021-09-29T11:04:43.305Z", 2) // Creation time is a random value pulled from the AWS DescribeImage docs

	tests := []struct {
		desc                   string
		checkError             require.ErrorAssertionFunc
		doDryRun               bool
		image                  imageFixture
		mockDeregisterImage    func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
		shouldIgnoreSpaceCheck bool
	}{
		{
			desc:  "fail if API call errors",
			image: oldImage,
			mockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
				return nil, trace.Errorf("some API call error")

			},
			checkError:             require.Error,
			shouldIgnoreSpaceCheck: true,
		},
		{
			desc:  "no error when API call does not error",
			image: oldImage,
			mockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
				return &ec2.DeregisterImageOutput{}, nil
			},
		},
		{
			desc:  "requests as a dry run when set in the application",
			image: oldImage,
			mockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
				if !*params.DryRun {
					return nil, trace.Errorf("API call was not set to a do dry run")
				}

				return &ec2.DeregisterImageOutput{}, nil
			},
			doDryRun: true,
		},
		{
			desc:  "requests as not a dry run when not set in the application",
			image: oldImage,
			mockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
				if params.DryRun != nil && *params.DryRun {
					return nil, trace.Errorf("API call was set to a do dry run")
				}

				return &ec2.DeregisterImageOutput{}, nil
			},
			doDryRun: false,
		},
		{
			desc:  "do not remove new images",
			image: generateImageFixture("new image", time.Now().AddDate(0, 0, -29).Format(time.RFC3339), 1),
			mockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
				return nil, trace.Errorf("The new image was deleted but should not have been")
			},
			shouldIgnoreSpaceCheck: true,
		},
	}

	for _, test := range tests {
		if test.checkError == nil {
			test.checkError = require.NoError
		}

		t.Run(test.desc, func(t *testing.T) {
			// Setup the application instance
			application := &ApplicationInstance{
				shouldDoDryRun: test.doDryRun,
				ec2ClientGenerator: func(cfg *aws.Config) EC2API {
					return &MockEC2API{
						MockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
							// Do nothing, just don't error
							return &ec2.DeleteSnapshotOutput{}, nil
						},
						MockDeregisterImage: test.mockDeregisterImage,
					}
				},
			}

			// Run the function under test
			recoveredSpace, err := application.cleanupImageIfOld(context.Background(), application.ec2ClientGenerator(nil), test.image.Image)

			// Verify the results
			test.checkError(t, err)
			if !test.shouldIgnoreSpaceCheck {
				require.Equal(t, test.image.totalSize, recoveredSpace, "the recovered space did not match the expected recovered space")
			}
		})
	}
}

func TestCleanupRegion(t *testing.T) {
	tests := []struct {
		desc          string
		imageFixtures []imageFixture
		regionName    string
	}{
		{
			desc:       "passes with no old images",
			regionName: "us-east-1",
		},
		{
			desc: "passes with one old image",
			imageFixtures: []imageFixture{
				generateImageFixture("image 1", "2021-09-29T11:04:43.305Z", 1),
			},
			regionName: "us-east-2",
		},
		{
			desc: "passes with many old images",
			imageFixtures: []imageFixture{
				generateImageFixture("image 1", "2021-09-29T11:04:43.305Z", 1),
				generateImageFixture("image 2", "2021-09-29T11:04:43.305Z", 2),
				generateImageFixture("image 3", "2021-09-29T11:04:43.305Z", 3),
				generateImageFixture("image 4", "2021-09-29T11:04:43.305Z", 4),
				generateImageFixture("image 5", "2021-09-29T11:04:43.305Z", 5),
				generateImageFixture("image 6", "2021-09-29T11:04:43.305Z", 6),
			},
			regionName: "us-west-1",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			var ec2ClientProvidedConfig *aws.Config

			// Setup the for the test
			application := &ApplicationInstance{
				ec2ClientGenerator: func(cfg *aws.Config) EC2API {
					ec2ClientProvidedConfig = cfg

					return &MockEC2API{
						MockDescribeImages: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
							images := make([]ec2types.Image, 0, len(test.imageFixtures))
							for _, imageFixture := range test.imageFixtures {
								images = append(images, imageFixture.Image)
							}

							return &ec2.DescribeImagesOutput{
								Images: images,
							}, nil
						},
						MockDeregisterImage: func(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
							return &ec2.DeregisterImageOutput{}, nil
						},
						MockDeleteSnapshot: func(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
							return &ec2.DeleteSnapshotOutput{}, nil
						},
					}
				},
			}

			sizeOfAllImages := int32(0)
			for _, image := range test.imageFixtures {
				sizeOfAllImages += image.totalSize
			}

			// Run the function under test
			recoveredSpace, imagesDeleted, err := application.cleanupRegion(context.Background(), aws.Config{}, test.regionName)

			// Verify the results
			require.NoError(t, err)
			require.Equal(t, sizeOfAllImages, recoveredSpace, "the recovered space did not match the expected recovered space")
			require.Equal(t, len(test.imageFixtures), imagesDeleted, "the number of deleted images did not match the expected number of deleted images")
			require.NotNil(t, ec2ClientProvidedConfig, "the function did not provide the AWS configuration to the client generator")
			require.Equal(t, ec2ClientProvidedConfig.Region, test.regionName, "the region was not set to the expected value on the AWS configuration")
		})
	}
}

type imageFixture struct {
	ec2types.Image
	totalSize int32
}

func generateImageFixture(name, creationDate string, snapshotCount int) imageFixture {
	image := imageFixture{
		Image: ec2types.Image{
			Name: &name,
		},
	}

	if creationDate != "" {
		image.CreationDate = &creationDate
	}

	for i := 0; i < snapshotCount; i++ {
		snapshotId := fmt.Sprintf("snap-%0x", i)
		snapshotSize := int32(1) << i
		image.BlockDeviceMappings = append(image.BlockDeviceMappings, ec2types.BlockDeviceMapping{
			Ebs: &ec2types.EbsBlockDevice{
				SnapshotId: &snapshotId,
				VolumeSize: &snapshotSize,
			},
		})
		image.totalSize += snapshotSize
	}

	return image
}
