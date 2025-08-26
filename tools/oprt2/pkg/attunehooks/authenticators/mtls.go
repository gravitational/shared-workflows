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
	"net"
	"net/url"
	"os/exec"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/commandrunner"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/proxy"
)

// MTLSAuthenticator authenticates with the Attune control plane with mTLS authentication.
type MTLSAuthenticator struct {
	attuneEndpointHost string
	attuneEndpointPort string
	certprovider       certprovider.Provider

	// State vars
	stopProxy    func() error
	proxyAddress string
}

var _ commandrunner.SetupHook = &MTLSAuthenticator{}
var _ commandrunner.CommandHook = &MTLSAuthenticator{}
var _ commandrunner.CleanupHook = &MTLSAuthenticator{}

func NewMutualTLSAuthenticator(attuneEndpoint string, certprovider certprovider.Provider) (mtlsa *MTLSAuthenticator, err error) {
	mtlsa = &MTLSAuthenticator{
		certprovider: certprovider,
	}

	attuneEndpointURL, err := url.Parse(attuneEndpoint)
	if err == nil {
		mtlsa.attuneEndpointHost = attuneEndpointURL.Hostname()
		if mtlsa.attuneEndpointHost == "" {
			return nil, fmt.Errorf("the Attune endpoint does not contain a hostname: %q", attuneEndpoint)
		}

		mtlsa.attuneEndpointPort = attuneEndpointURL.Port()
		if mtlsa.attuneEndpointPort != "" {
			return mtlsa, nil
		}

		switch attuneEndpointURL.Scheme {
		case "https":
			mtlsa.attuneEndpointPort = "443"
			return mtlsa, nil
		case "http":
			mtlsa.attuneEndpointPort = "80"
			return mtlsa, nil
		}
	}

	if host, port, err := net.SplitHostPort(attuneEndpoint); err == nil {
		mtlsa.attuneEndpointHost = host
		mtlsa.attuneEndpointPort = port
		return mtlsa, nil
	}

	return nil, fmt.Errorf("failed to parse Attune endpoint: %q", attuneEndpoint)
}

func (mtlsa *MTLSAuthenticator) Name() string {
	return "mTLS"
}

func (mtlsa *MTLSAuthenticator) Setup(ctx context.Context) error {
	// Start a TCP proxy
	proxyServeCtx, cancelProxyServe := context.WithCancel(ctx)
	t2tp := proxy.NewTCP2TLSProxy(mtlsa.attuneEndpointHost, mtlsa.attuneEndpointPort, proxy.WithClientCertificateProvider(mtlsa.certprovider))
	proxyServeErr := make(chan error)
	go func() {
		defer close(proxyServeErr)
		proxyServeErr <- t2tp.ListenAndServe(proxyServeCtx)
	}()

	mtlsa.stopProxy = func() error {
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
	mtlsa.proxyAddress = proxyAddress.String()

	return nil
}

func (mtlsa *MTLSAuthenticator) Command(_ context.Context, cmd *exec.Cmd) error {
	cmd.Env = append(
		cmd.Env,
		// This value is only here because the Attune CLI requires it to be set. It is
		// meaningless, and the ingres gateway replaces on backend requests.
		"ATTUNE_API_TOKEN=dummy-value",
		// This must be HTTP to avoid dealing with trust issues, and because the proxy is
		// only aware of TCP, downwards. The proxy always binds to localhost anyway, so
		// there isn't a security risk here that we are concerned about.
		"ATTUNE_API_ENDPOINT=http://"+mtlsa.proxyAddress,
	)
	return nil
}

func (mtlsa *MTLSAuthenticator) Cleanup(ctx context.Context) error {
	if mtlsa.stopProxy == nil {
		return nil
	}

	if err := mtlsa.stopProxy(); err != nil {
		return fmt.Errorf("failed to stop TCP2TLS proxy: %w", err)
	}

	return nil
}
