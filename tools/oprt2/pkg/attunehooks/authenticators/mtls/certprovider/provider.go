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

package certprovider

import (
	"context"
	"crypto/tls"
	"errors"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/config"
)

// Provider provides a certificate for client authentication.
type Provider interface {
	// Gets a keypair for use with mTLS authentication.
	GetClientCertificate(context.Context) (*tls.Certificate, error)
}

// FromConfig builds a cert provider from the provided config.
func FromConfig(config config.CertificateProvider) (Provider, error) {
	return nil, errors.New("not implemented")
}
