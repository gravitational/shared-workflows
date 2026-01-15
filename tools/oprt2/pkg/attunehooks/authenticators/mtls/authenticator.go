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
	"net"
	"net/url"
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls/proxy"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// Authenticator authenticates with the Attune control plane with mTLS authentication.
// It does this by creating a local TCP proxy that wraps the connection in TLS, forwarding
// it to a reverse proxy in front of Attune. The reverse proxy handles authentication, and
// strips the TLS layer.
type Authenticator struct {
	attuneEndpointHost string
	attuneEndpointPort string
	certprovider       certprovider.Provider
	logger             *slog.Logger

	// State vars
	stopProxy    func() error
	proxyAddress string
}

var _ commandrunner.Hook = (*Authenticator)(nil)

// Authenticator creates a new Authenticator.
func NewAuthenticator(ctx context.Context, attuneEndpoint string, certprovider certprovider.Provider, opts ...AuthenticatorOption) (a *Authenticator, err error) {
	a = &Authenticator{
		certprovider: certprovider,
		logger:       logging.DiscardLogger,
	}

	if err := a.setHostPort(attuneEndpoint); err != nil {
		return nil, err
	}

	for _, opt := range opts {
		opt(a)
	}

	if err := a.setup(ctx); err != nil {
		return nil, err
	}

	return a, nil
}

// Name is the name of the authenticator.
func (a *Authenticator) Name() string {
	return "mTLS"
}

func (a *Authenticator) setHostPort(attuneEndpoint string) error {
	attuneEndpointURL, err := url.Parse(attuneEndpoint)
	if err == nil {
		a.attuneEndpointHost = attuneEndpointURL.Hostname()
		if a.attuneEndpointHost == "" {
			return fmt.Errorf("the Attune endpoint does not contain a hostname: %q", attuneEndpoint)
		}

		a.attuneEndpointPort = attuneEndpointURL.Port()
		if a.attuneEndpointPort != "" {
			return nil
		}

		switch attuneEndpointURL.Scheme {
		case "https":
			a.attuneEndpointPort = "443"
			return nil
		case "http":
			a.attuneEndpointPort = "80"
			return nil
		}
	}

	if host, port, err := net.SplitHostPort(attuneEndpoint); err == nil {
		a.attuneEndpointHost = host
		a.attuneEndpointPort = port
		return nil
	}

	return fmt.Errorf("failed to parse Attune endpoint: %q", attuneEndpoint)
}

func (a *Authenticator) setup(ctx context.Context) error {
	// Start a TCP proxy
	proxyServeCtx, cancelProxyServe := context.WithCancel(ctx)
	t2tp := proxy.NewTCP2TLS(a.attuneEndpointHost, a.attuneEndpointPort, proxy.WithLogger(a.logger), proxy.WithClientCertificateProvider(a.certprovider))
	proxyServeErr := make(chan error)
	go func() {
		defer close(proxyServeErr)
		proxyServeErr <- t2tp.ListenAndServe(proxyServeCtx)
	}()

	a.stopProxy = func() error {
		cancelProxyServe()
		actualProxyServeErr := <-proxyServeErr
		if actualProxyServeErr != nil {
			actualProxyServeErr = fmt.Errorf("the TLS2TCP proxy failed while serving: %w", actualProxyServeErr)
		}
		return actualProxyServeErr
	}

	proxyAddress, err := t2tp.GetAddress(ctx)
	if err != nil {
		return fmt.Errorf("failed to get TLS2TCP listening address: %w", err)
	}
	a.proxyAddress = proxyAddress.String()

	return nil
}

// Command adds mTLS authentication to the Attune command.
func (a *Authenticator) Command(_ context.Context, cmd *exec.Cmd) error {
	cmd.Env = append(
		cmd.Env,
		// This value is only here because the Attune CLI requires it to be set. It is
		// meaningless, and the ingres gateway replaces on backend requests.
		"ATTUNE_API_TOKEN=dummy-value",
		// This must be HTTP to avoid dealing with trust issues, and because the proxy is
		// only aware of TCP, downwards. The proxy always binds to localhost anyway, so
		// there isn't a security risk here that we are concerned about.
		"ATTUNE_API_ENDPOINT=http://"+a.proxyAddress,
	)
	return nil
}

// Close closes the authenticator.
func (a *Authenticator) Close(ctx context.Context) error {
	if a.stopProxy == nil {
		return nil
	}

	if err := a.stopProxy(); err != nil {
		return fmt.Errorf("failed to stop TCP2TLS proxy: %w", err)
	}

	return nil
}
