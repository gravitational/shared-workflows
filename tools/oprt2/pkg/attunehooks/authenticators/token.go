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

package authenticators

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
)

// TokenAuthenticator authenticates with the Attune control plane with Attune-supported token authentication.
type TokenAuthenticator struct {
	attuneEndpoint string
	token          string
}

var _ commandrunner.CommandHook = &TokenAuthenticator{}

func NewTokenAuthenticator(attuneEndpoint, token string) (*TokenAuthenticator, error) {
	attuneEndpointURL, err := url.Parse(attuneEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid Attune endpoint URL %q: %w", attuneEndpoint, err)
	}

	if attuneEndpointURL.Hostname() == "" {
		return nil, fmt.Errorf("the Attune endpoint URL is missing a hostname: %q", attuneEndpoint)
	}

	return &TokenAuthenticator{
		attuneEndpoint: attuneEndpoint,
		token:          token,
	}, nil
}

func (ta *TokenAuthenticator) Name() string {
	return "token"
}

func (ta *TokenAuthenticator) Command(_ context.Context, cmd *exec.Cmd) error {
	cmd.Env = append(
		cmd.Env,
		"ATTUNE_API_TOKEN="+ta.token,
		"ATTUNE_API_ENDPOINT="+ta.attuneEndpoint,
	)
	return nil
}
