// Package images enumerates the AMIs an AWS account owns within a single region.
package images

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/shared-workflows/tools/amicleanup/models"
)

type imagesClient interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

// AWS returns CreationDate as a string in this layout (RFC3339 with milliseconds).
const awsCreationDateLayout = "2006-01-02T15:04:05.000Z"

// ImagesInRegion paginates DescribeImages with Owners=["self"] and returns
// every AMI the calling account owns in the given region. The Region field of
// each returned Image is populated from the region argument.
func ImagesInRegion(ctx context.Context, c imagesClient, region string) ([]models.Image, error) {
	var (
		out       []models.Image
		nextToken *string
	)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		page, err := c.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners:    []string{"self"},
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe images in %s: %w", region, err)
		}

		for _, img := range page.Images {
			out = append(out, convertImage(img, region))
		}

		if page.NextToken == nil {
			return out, nil
		}
		nextToken = page.NextToken
	}
}

func convertImage(in types.Image, region string) models.Image {
	out := models.Image{Region: region}
	if in.ImageId != nil {
		out.ID = *in.ImageId
	}

	if in.Name != nil {
		out.Name = *in.Name
	}

	if in.Public != nil {
		out.Public = *in.Public
	}

	if in.CreationDate != nil {
		if t, err := time.Parse(awsCreationDateLayout, *in.CreationDate); err == nil {
			out.CreationDate = t
		}
	}

	for _, bdm := range in.BlockDeviceMappings {
		if bdm.Ebs == nil || bdm.Ebs.SnapshotId == nil || *bdm.Ebs.SnapshotId == "" {
			continue
		}
		dev := models.BlockDevice{SnapshotID: *bdm.Ebs.SnapshotId}

		if bdm.DeviceName != nil {
			dev.DeviceName = *bdm.DeviceName
		}

		out.BlockDevices = append(out.BlockDevices, dev)
	}

	return out
}
