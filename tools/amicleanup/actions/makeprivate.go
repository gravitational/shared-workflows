package actions

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
)

// MakePrivateAction removes Group="all" launch permission, reverting an AMI
// to private. Idempotent: removing a permission that isn't there is a no-op
// at the AWS API level.
type MakePrivateAction struct{}

func NewMakePrivateAction() *MakePrivateAction { return &MakePrivateAction{} }

func (a *MakePrivateAction) Name() string { return NameMakePrivate }

func (a *MakePrivateAction) Describe(img models.Image) string {
	return fmt.Sprintf("make-private %s in %s", img.ID, img.Region)
}

func (a *MakePrivateAction) Apply(ctx context.Context, c ec2iface.EC2API, img models.Image) error {
	_, err := c.ModifyImageAttribute(ctx, &ec2.ModifyImageAttributeInput{
		ImageId: &img.ID,
		LaunchPermission: &types.LaunchPermissionModifications{
			Remove: []types.LaunchPermission{{Group: types.PermissionGroupAll}},
		},
	})
	if err != nil {
		return fmt.Errorf("make image private %s: %w", img.ID, err)
	}

	return nil
}
