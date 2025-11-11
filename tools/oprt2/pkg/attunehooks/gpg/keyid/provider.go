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

package keyid

import (
	"context"
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
)

// Provider is a [commandrunner.Hook] that provides access to a GPG key via a key
// that has already been loaded into the GPG home directory.
type Provider struct {
	keyID      string
	gpgHomeDir string
}

var _ commandrunner.Hook = (*Provider)(nil)

// NewProvider creates a new Provider.
func NewProvider(opts ...ProviderOption) *Provider {
	p := &Provider{}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Name implements the [commandrunner.Hook] interface.
func (p *Provider) Name() string {
	return "GPG key ID provider"
}

// Command implements the [commandrunner.Hook] interface.
func (p *Provider) Command(_ context.Context, cmd *exec.Cmd) error {
	providerFlags := make([]string, 0, 4)

	if p.gpgHomeDir != "" {
		providerFlags = append(providerFlags, "--gpg-home-dir", p.gpgHomeDir)
	}

	if p.keyID != "" {
		providerFlags = append(providerFlags, "--key-id", p.keyID)
	}

	if len(cmd.Args) == 0 {
		cmd.Args = providerFlags
		return nil
	}

	// Split the file path from the flags
	filePath := cmd.Args[len(cmd.Args)-1]

	var flags []string
	if len(cmd.Args) > 1 {
		flags = cmd.Args[0 : len(cmd.Args)-2]
	}

	// Args = flags + provider flags + file name
	args := make([]string, len(flags)+len(providerFlags)+1)
	args = append(args, flags...)
	args = append(args, providerFlags...)
	args = append(args, filePath)

	cmd.Args = args
	return nil
}

// Close implements the [commandrunner.Hook] interface.
func (p *Provider) Close(_ context.Context) error {
	return nil
}
