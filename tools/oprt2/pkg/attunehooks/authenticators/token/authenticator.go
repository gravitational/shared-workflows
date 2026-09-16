/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package token

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
)

// Authenticator authenticates with the Attune control plane with Attune-supported token authentication.
type Authenticator struct {
	attuneEndpoint string
	token          string
}

var _ commandrunner.Hook = (*Authenticator)(nil)

// NewAuthenticator creates a new Authenticator
func NewAuthenticator(attuneEndpoint, token string) (*Authenticator, error) {
	attuneEndpointURL, err := url.Parse(attuneEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid Attune endpoint URL %q: %w", attuneEndpoint, err)
	}

	if attuneEndpointURL.Hostname() == "" {
		return nil, fmt.Errorf("the Attune endpoint URL is missing a hostname: %q", attuneEndpoint)
	}

	return &Authenticator{
		attuneEndpoint: attuneEndpoint,
		token:          token,
	}, nil
}

// Name is the name of the authenticator.
func (a *Authenticator) Name() string {
	return "token"
}

// Command adds token authentication to the Attune command.
func (a *Authenticator) Command(_ context.Context, cmd *exec.Cmd) error {
	cmd.Env = append(
		cmd.Env,
		"ATTUNE_API_TOKEN="+a.token,
		"ATTUNE_API_ENDPOINT="+a.attuneEndpoint,
	)
	return nil
}

// Close closes the authenticator.
func (a *Authenticator) Close(_ context.Context) error {
	return nil
}
