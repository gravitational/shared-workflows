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

package mtls

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

func FromConfig(ctx context.Context, config *config.MTLSAuthenticator, logger *slog.Logger) (*Authenticator, error) {
	certProvider, err := certprovider.FromConfig(config.CertificateSource)
	if err != nil {
		return nil, fmt.Errorf("failed to get mTLS certificate source: %w", err)
	}

	authenticator, err := NewAuthenticator(ctx, config.Endpoint, certProvider, WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("failed to create mTLS authenticator: %w", err)
	}

	return authenticator, nil
}
