package actions

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
)

// MakePublicAction grants Group="all" launch permission, making the AMI
// public. Public AMIs can be launched by any AWS account and may carry
// security/compliance implications — the manager prompts for confirmation
// before applying this action in non-dry-run mode.
type MakePublicAction struct{}

func NewMakePublicAction() *MakePublicAction { return &MakePublicAction{} }

func (a *MakePublicAction) Name() string { return NameMakePublic }

func (a *MakePublicAction) Describe(img models.Image) string {
	return fmt.Sprintf("make-public %s in %s", img.ID, img.Region)
}

func (a *MakePublicAction) Apply(ctx context.Context, c ec2iface.EC2API, img models.Image) error {
	_, err := c.ModifyImageAttribute(ctx, &ec2.ModifyImageAttributeInput{
		ImageId: &img.ID,
		LaunchPermission: &types.LaunchPermissionModifications{
			Add: []types.LaunchPermission{{Group: types.PermissionGroupAll}},
		},
	})
	if err != nil {
		return fmt.Errorf("make image public %s: %w", img.ID, err)
	}

	return nil
}
