package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
)

// DeprecateAction marks AMIs as deprecated effective at At (defaulting to "now"
// when zero). It uses ec2:EnableImageDeprecation, which is reversible via
// DisableImageDeprecation.
type DeprecateAction struct{}

// NewDeprecateAction constructs a DeprecateAction. A zero `at` means "now",
// resolved at Apply time so each call gets a current timestamp.
func NewDeprecateAction() *DeprecateAction {
	return &DeprecateAction{}
}

func (a *DeprecateAction) Name() string { return NameDeprecate }

func (a *DeprecateAction) Describe(img models.Image) string {
	return fmt.Sprintf("deprecate %s in %s", img.ID, img.Region)
}

func (a *DeprecateAction) Apply(ctx context.Context, c ec2iface.EC2API, img models.Image) error {
	_, err := c.EnableImageDeprecation(ctx, &ec2.EnableImageDeprecationInput{
		ImageId: &img.ID,
		// Pick a time in the future because the API requires the current or future time. This should
		// be sufficiently large to cover clock drift, long running API calls, etc
		DeprecateAt: new(time.Now().Add(10 * time.Minute).UTC()),
	})
	if err != nil {
		return fmt.Errorf("enable image deprecation %s: %w", img.ID, err)
	}

	return nil
}
