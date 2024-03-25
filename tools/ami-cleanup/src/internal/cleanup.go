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

package internal

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/account"
	accountTypes "github.com/aws/aws-sdk-go-v2/service/account/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/gravitational/trace"
	"github.com/relvacode/iso8601"
)

type ApplicationInstance struct {
	shouldDoDryRun bool

	// Dependency injection for testing
	accountClientGenerator func(cfg *aws.Config) IAccountApi
	ec2ClientGenerator     func(cfg *aws.Config) IEc2Api
}

// Creates a new instance of the tool
func NewApplicationInstance(doDryRun bool) *ApplicationInstance {
	return &ApplicationInstance{
		shouldDoDryRun:         doDryRun,
		accountClientGenerator: NewAccountApi,
		ec2ClientGenerator:     NewEc2Api,
	}
}

// Performs cleanup
func (ai *ApplicationInstance) Run(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return trace.Wrap(err, "failed to load AWS credentials")
	}

	enabledRegions, err := ai.getEnabledRegions(ctx, ai.accountClientGenerator(&cfg))
	if err != nil {
		return trace.Wrap(err, "failed to get enabled regions")
	}

	if len(enabledRegions) == 0 {
		return nil
	}

	totalSpaceRecovered := int32(0)
	totalImagesDeleted := 0
	// This would probably run much faster with channels/concurrency, but there isn't much
	// need with how often this is expected to be ran
	for _, enabledRegion := range enabledRegions {
		spaceRecovered, imagesDeleted, err := ai.cleanupRegion(ctx, cfg, *enabledRegion.RegionName)
		if err != nil {
			return trace.Wrap(err, "failed to clean up images in region %q", *enabledRegion.RegionName)
		}

		totalSpaceRecovered += spaceRecovered
		totalImagesDeleted += imagesDeleted
	}

	log.Printf("Deleted %d GiB of %d images across %d regions", totalSpaceRecovered, totalImagesDeleted, len(enabledRegions))
	return nil
}

// Cleans up the images in the given region. Returns the total amount of space recovered, number of images deleted,
// and any error.
func (ai ApplicationInstance) cleanupRegion(ctx context.Context, cfg aws.Config, regionName string) (int32, int, error) {
	// A new client must be created for each region
	cfg.Region = regionName
	ec2Client := ai.ec2ClientGenerator(&cfg)

	log.Printf("Deleting images in %q", regionName)
	devImages, err := ai.getDevImagesInRegion(ctx, ec2Client)
	if err != nil {
		return 0, 0, trace.Wrap(err, "failed to get a list of dev images for %q", regionName)
	}

	var totalSpaceRecovered int32
	totalImagesDeleted := 0
	totalSnapshotCount := 0
	for _, devImage := range devImages {
		imageSpace, err := ai.cleanupImageIfOld(ctx, ec2Client, devImage)
		if err != nil {
			return 0, 0, trace.Wrap(err, "failed to cleanup dev image %q", devImage.Name)
		}

		if imageSpace == 0 {
			continue
		}

		totalSpaceRecovered += imageSpace
		totalImagesDeleted++
		totalSnapshotCount += len(devImage.BlockDeviceMappings)
	}

	return totalSpaceRecovered, totalImagesDeleted, nil
}

func (ai *ApplicationInstance) getEnabledRegions(ctx context.Context, client IAccountApi) ([]accountTypes.Region, error) {
	return getAllWithPagination(
		func(previousToken *string) (*string, []accountTypes.Region, error) {
			results, err := client.ListRegions(ctx, &account.ListRegionsInput{
				RegionOptStatusContains: []accountTypes.RegionOptStatus{
					accountTypes.RegionOptStatusEnabled,
					accountTypes.RegionOptStatusEnabledByDefault,
				},
				NextToken: previousToken,
			})
			if err != nil {
				return nil, nil, trace.Wrap(err, "failed to request enabled regions")
			}

			return results.NextToken, results.Regions, nil
		},
	)
}

func (ai *ApplicationInstance) getDevImagesInRegion(ctx context.Context, client IEc2Api) ([]ec2Types.Image, error) {
	// These are a weird language workaround for getting a pointer to a boolean literal
	includeTrue := true
	nameFilterName := "name"
	stateFilterName := "state"
	requestInput := &ec2.DescribeImagesInput{
		Filters: []ec2Types.Filter{
			{
				Name:   &nameFilterName,
				Values: []string{"*dev*"},
			},
			{
				Name:   &stateFilterName,
				Values: []string{string(ec2Types.ImageStateAvailable)},
			},
		},
		IncludeDeprecated: &includeTrue,
		IncludeDisabled:   &includeTrue,
		Owners:            []string{"self"},
	}

	return getAllWithPagination(
		func(previousToken *string) (*string, []ec2Types.Image, error) {

			requestInput.NextToken = previousToken
			results, err := client.DescribeImages(ctx, requestInput)
			if err != nil {
				return nil, nil, trace.Wrap(err, "failed to request a dev images")
			}

			return results.NextToken, results.Images, nil
		},
	)
}

// Repeatedly calls `action` until the returned `nextToken` is null, and accumulates the results.
// Useful for AWS API calls who's results may be paginated.
func getAllWithPagination[T any](action func(previousToken *string) (nextToken *string, results []T, err error)) ([]T, error) {
	var previousToken *string
	var results []T
	for {
		nextToken, newResults, err := action(previousToken)
		if err != nil {
			return results, trace.Wrap(err, "failed to get the next set of results")
		}

		results = append(results, newResults...)
		if nextToken == nil {
			return results, nil
		}
		previousToken = nextToken
	}
}

func (ai *ApplicationInstance) cleanupImageIfOld(ctx context.Context, client IEc2Api, image ec2Types.Image) (int32, error) {
	creationDate, err := iso8601.ParseString(*image.CreationDate)
	if err != nil {
		return 0, trace.Wrap(err, "failed to parse image %q creation timestamp %q as an ISO 8601 value", *image.Name, *image.CreationDate)
	}

	// If the image is less than a month old, don't do anything
	imageAge := time.Since(creationDate)
	if imageAge <= 30*24*time.Hour {
		return 0, nil
	}

	log.Printf("\tDeleting %s old AMI %q", imageAge.String(), *image.Name)
	_, err = client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
		ImageId: image.ImageId,
		DryRun:  &ai.shouldDoDryRun,
	})
	if err != nil && (!IsDryRunError(err) || !ai.shouldDoDryRun) {
		return 0, trace.Wrap(err, "failed to deregister image %q", *image.Name)
	}

	deletedImageSize, err := ai.deleteSnapshotsForImage(ctx, client, image)
	if err != nil {
		return 0, trace.Wrap(err, "failed to delete all snapshots for image %q", *image.Name)
	}
	return deletedImageSize, nil
}

// Deletes all the snapshots for the given image, returning their cumulative snapshot size in GiB.
func (ai *ApplicationInstance) deleteSnapshotsForImage(ctx context.Context, client IEc2Api, image ec2Types.Image) (int32, error) {
	deletedImageSize := int32(0)
	for _, blockDevice := range image.BlockDeviceMappings {
		log.Printf("\t\tDeleting snapshot %q for AMI %q", *blockDevice.Ebs.SnapshotId, *image.Name)
		_, err := client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: blockDevice.Ebs.SnapshotId,
			DryRun:     &ai.shouldDoDryRun,
		})

		if err != nil && (!IsDryRunError(err) || !ai.shouldDoDryRun) {
			return 0, trace.Wrap(err, "failed to delete snapshot %q for AMI %q, please check for hanging snapshots",
				*blockDevice.Ebs.SnapshotId, *image.Name)
		}

		deletedImageSize += *blockDevice.Ebs.VolumeSize
	}

	return deletedImageSize, nil
}
