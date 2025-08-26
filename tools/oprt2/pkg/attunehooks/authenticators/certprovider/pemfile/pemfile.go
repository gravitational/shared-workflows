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

package pemfile

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/certprovider"
)

// PEMFileProvider provides a certificate via a pair of PEM-encoded public/private key files.
// Supports external certificate renewers (e.g. k8s + cert-manager).
type PEMFileProvider struct {
	*certprovider.RenewableCertProvider

	privateKeyPath string
	publicKeyPaths []string
}

var _ certprovider.Provider = &PEMFileProvider{}

type PEMFileProviderOpt func(*PEMFileProvider)

// WithCertChain configured the provider to also provide intermediary certs in the chain.
func WithCertChain(certPaths ...string) PEMFileProviderOpt {
	return func(pemp *PEMFileProvider) {
		pemp.publicKeyPaths = append(pemp.publicKeyPaths, certPaths...)
	}
}

func NewPEMProvider(privateKeyPath string, publicKeyPath string, opts ...PEMFileProviderOpt) *PEMFileProvider {
	pemp := &PEMFileProvider{
		privateKeyPath: privateKeyPath,
		publicKeyPaths: []string{publicKeyPath},
	}

	for _, opt := range opts {
		opt(pemp)
	}

	pemp.RenewableCertProvider = certprovider.NewRenewableCertProvider(2*time.Minute, pemp.getClientCertificate)

	return pemp
}

// getClientCertificate reads the cert files from the disk. These can change if an external process
// modifies them.
func (pemp *PEMFileProvider) getClientCertificate(_ context.Context) (*tls.Certificate, error) {
	// Read the private key
	privateKeyBytes, err := os.ReadFile(pemp.privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key from %q: %w", pemp.privateKeyPath, err)
	}

	// Read the public key + any certs provided for the chain
	totalByteCount := 0
	publicKeysBytes := make([][]byte, 0, len(pemp.publicKeyPaths))
	for _, publicKeyPath := range pemp.publicKeyPaths {
		publicKeyBytes, err := os.ReadFile(publicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read public key file from %q: %w", publicKeyPath, err)
		}

		totalByteCount += len(publicKeyBytes)
		publicKeysBytes = append(publicKeysBytes, publicKeyBytes)
	}

	combinedPublicKeyBytes := make([]byte, 0, totalByteCount)
	for _, publicKeyBytes := range publicKeysBytes {
		combinedPublicKeyBytes = append(combinedPublicKeyBytes, publicKeyBytes...)
	}

	// Build the full cert
	cert, err := tls.X509KeyPair(combinedPublicKeyBytes, privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chain, public key, and/or private key: %w", err)
	}

	return &cert, nil
}
