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
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// RenewableCertProvider is a "wrapper" implementation of a certificate provider that supports
// certificate renewal. This is needed to support long-running operations like migrations and DR.
type RenewableCertProvider struct {
	getClientCertificate func(context.Context) (*tls.Certificate, error)

	// state vars
	renewLock            sync.RWMutex
	currentCert          *tls.Certificate
	shouldAttemptRenewal bool
	timeToRenewBefore    time.Duration
}

var _ Provider = &RenewableCertProvider{}

func NewRenewableCertProvider(timeToRenewBefore time.Duration, getClientCertificate func(context.Context) (*tls.Certificate, error)) *RenewableCertProvider {
	return &RenewableCertProvider{
		timeToRenewBefore:    timeToRenewBefore,
		getClientCertificate: getClientCertificate,
	}
}

func (rcp *RenewableCertProvider) GetClientCertificate(ctx context.Context) (*tls.Certificate, error) {
	// Attempt to retrieve the cached certificate if it is valid, otherwise, renew it. Handle the case where
	// concurrent callers are attempting to retrieve an invalid cert, and both attempt renewal.
	for {
		// TODO these functions will ignore context cancellation, so a single misbehaving call to the underlying
		// getClientCertificate will block all other calls even after ctx is cancelled. This could pose shutdown
		// problems.

		cert, err := rcp.tryRetrieveCert(ctx)
		if err != nil {
			return nil, fmt.Errorf("faield to retrieve existing cert: %w", err)
		}

		if cert != nil {
			return cert, nil
		}

		// Attempt to get a new cert if one was unable to be retrieved
		if err := rcp.tryRenewCert(ctx); err != nil {
			return nil, fmt.Errorf("failed to renew certificate: %w", err)
		}

		// The certificate should be renewed at this point by the time the mutex is available for read locking
	}
}

// tryRetrieveCert attempts to retrieve the existing cert, if it is currently valid.
// Returns nil if the certificate should be renewed.
func (rcp *RenewableCertProvider) tryRetrieveCert(ctx context.Context) (*tls.Certificate, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	rcp.renewLock.RLock()
	defer rcp.renewLock.Unlock()

	// Return the current cert if it's been set and if it isn't about to expire
	if rcp.currentCert != nil && rcp.currentCert.Leaf.NotAfter.Add(rcp.timeToRenewBefore).After(time.Now()) {
		// This ptr copy should be fine because latestCert is replaced entirely when it is updated,
		// never modified.
		return rcp.currentCert, nil
	}

	if !rcp.shouldAttemptRenewal {
		return nil, fmt.Errorf("cert is expired and cannot be renewed")
	}

	return nil, nil
}

// tryRenewCert attempts to use the underlying provider to renew the certificate. If it cannot get the lock,
// then the function immediately returns, allowing the caller to attempt certificate retrieval again.
func (rcp *RenewableCertProvider) tryRenewCert(ctx context.Context) error {
	// Try to switch to a write lock. If it is already held, another caller is attempting to update the cert
	// so cert retrieval should be retried instead of renewal.
	if !rcp.renewLock.TryLock() {
		return nil
	}
	defer rcp.renewLock.Unlock()

	// Handle context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	logging.FromCtx(ctx).DebugContext(ctx, "attempting to renew certificate")

	// Do the actual renewal
	newCert, err := rcp.getClientCertificate(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cert for cert provider: %w", err)
	}

	// Parse if needed. The x509 package does this lazily.
	if newCert.Leaf == nil {
		if len(newCert.Certificate) == 0 {
			return fmt.Errorf("retrieved certificate has no public key/leaf cert")
		}

		var err error
		newCert.Leaf, err = x509.ParseCertificate(newCert.Certificate[0])
		if err != nil {
			return fmt.Errorf("failed to parse leaf cert: %w", err)
		}
	}

	// Verify whether or not the certificate actually changed
	if rcp.currentCert != nil && rcp.currentCert.Leaf != nil && rcp.currentCert.Leaf.SerialNumber == newCert.Leaf.SerialNumber {
		// Cert didn't change, so stop checking for renewals
		rcp.shouldAttemptRenewal = false
		return nil
	}
	rcp.currentCert = newCert

	return nil
}
