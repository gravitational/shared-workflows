package authenticators

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/token"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// FromConfig builds an Attune authenticator hook from the provided config.
func FromConfig(ctx context.Context, config config.Authenticator, logger *slog.Logger) (commandrunner.Hook, error) {
	switch {
	case config.MTLS != nil:
		return mtls.FromConfig(ctx, config.MTLS, logger)
	case config.Token != nil:
		return token.FromConfig(config.Token)
	default:
		return nil, errors.New("no or unknown Attune authenticator specified")
	}
}
