// Package actions defines the lifecycle operations amicleanup applies to AMIs.
//
// Each action is a struct that implements the Action interface; the registry
// in this file is the only place flag parsing in main.go needs to know about
// concrete action types.
package actions

import (
	"context"
	"fmt"

	"github.com/shared-workflows/tools/amicleanup/ec2iface"
	"github.com/shared-workflows/tools/amicleanup/models"
)

// Action lifecycle operation applied to a single AMI.
//
// Implementations always invoke the EC2API methods unconditionally: dry-run
// behaviour is enforced by wrapping the EC2API in client.NewReadOnly so that
// writes become no-ops. This keeps Action implementations from having to
// branch on a dry-run flag, which is easy to forget.
type Action interface {
	// Name is the canonical short name (matches the --action flag value).
	Name() string
	// Describe returns a human-readable preview line, e.g. "would deprecate ami-… in us-east-1".
	Describe(img models.Image) string
	// Apply performs the action against the given image.
	Apply(ctx context.Context, c ec2iface.EC2API, img models.Image) error
}

// Names of the four supported actions. Kept as constants so tests and the
// flag parser don't have to repeat the string literals.
const (
	NameDeprecate   = "deprecate"
	NameMakePublic  = "make-public"
	NameMakePrivate = "make-private"
	NameDelete      = "delete"
)

// NewByName constructs the action selected by --action. Returns a clear error
// for unknown actions so main.go can surface it to the user.
func NewByName(name string) (Action, error) {
	switch name {
	case NameDeprecate:
		return NewDeprecateAction(), nil
	case NameMakePublic:
		return NewMakePublicAction(), nil
	case NameMakePrivate:
		return NewMakePrivateAction(), nil
	case NameDelete:
		return NewDeleteAction(), nil
	default:
		return nil, fmt.Errorf("unknown action %q (valid: %s, %s, %s, %s)", name, NameDeprecate, NameMakePublic, NameMakePrivate, NameDelete)
	}
}
