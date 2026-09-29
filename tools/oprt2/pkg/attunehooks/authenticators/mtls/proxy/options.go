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

package proxy

import (
	"log/slog"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

type TCP2TLSOption func(*TCP2TLS)

// WithClientCertificateProvider configures the proxy to use mTLS authentication via certs provided by the provider.
func WithClientCertificateProvider(provider certprovider.Provider) TCP2TLSOption {
	return func(t2t *TCP2TLS) {
		t2t.clientCertProvider = provider
	}
}

// WithLogger configures the proxy with the provided logger.
func WithLogger(logger *slog.Logger) TCP2TLSOption {
	return func(t2t *TCP2TLS) {
		if logger == nil {
			logger = logging.DiscardLogger
		}
		t2t.logger = logger
	}
}
