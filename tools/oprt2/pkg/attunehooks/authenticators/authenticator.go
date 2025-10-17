package authenticators

import (
	"errors"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

func FromConfig(config config.Authenticator) (commandrunner.Hook, error) {
	return nil, errors.New("not implemented")
}
