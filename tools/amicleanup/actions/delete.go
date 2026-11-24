package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
)

// DeleteAction deregisters an AMI and then deletes every EBS snapshot backing
// it. Snapshot deletion failures do NOT undo the deregister (which is already
// committed) but ARE reported as the entry's error so an operator can clean
// orphans up later.
type DeleteAction struct{}

func NewDeleteAction() *DeleteAction { return &DeleteAction{} }

func (a *DeleteAction) Name() string { return NameDelete }

func (a *DeleteAction) Describe(img models.Image) string {
	if len(img.BlockDevices) == 0 {
		return fmt.Sprintf("delete %s in %s", img.ID, img.Region)
	}

	snaps := make([]string, len(img.BlockDevices))
	for i, b := range img.BlockDevices {
		snaps[i] = b.SnapshotID
	}

	return fmt.Sprintf("delete %s in %s (and snapshots %s)", img.ID, img.Region, strings.Join(snaps, ","))
}

func (a *DeleteAction) Apply(ctx context.Context, c ec2iface.EC2API, img models.Image) error {
	if _, err := c.DeregisterImage(ctx, &ec2.DeregisterImageInput{ImageId: &img.ID}); err != nil {
		return fmt.Errorf("deregister image %s: %w", img.ID, err)
	}

	var snapErrs []error
	for _, bd := range img.BlockDevices {
		if bd.SnapshotID == "" {
			continue
		}

		if _, err := c.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: &bd.SnapshotID}); err != nil {
			snapErrs = append(snapErrs, fmt.Errorf("delete snapshot %s: %w", bd.SnapshotID, err))
		}
	}

	if len(snapErrs) > 0 {
		return fmt.Errorf("ami %s deregistered but snapshot cleanup failed: %w", img.ID, errors.Join(snapErrs...))
	}

	return nil
}
