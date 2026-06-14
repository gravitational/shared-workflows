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
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/attunehooks/authenticators/mtls/certprovider"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/ctxcopy"
	"github.com/gravitational/shared-workflows/tools/oprt2/pkg/logging"
)

// TCP2TLS accepts TCP connections and wraps them in a (m)TLS tunnel to a specific destination.
type TCP2TLS struct {
	listeningAddress   *net.TCPAddr
	started            chan struct{}
	closeStartedChan   func() // Used to ensure that `close(started)` is only called once
	destinationHost    string
	destinationPort    string
	clientCertProvider certprovider.Provider
	logger             *slog.Logger
}

// NewTCP2TLS creates a new TCP2TLS proxy instance.
func NewTCP2TLS(destinationHost, destinationPort string, opts ...TCP2TLSOption) *TCP2TLS {
	started := make(chan struct{})
	t2t := &TCP2TLS{
		started:          started,
		closeStartedChan: sync.OnceFunc(func() { close(started) }),
		destinationHost:  destinationHost,
		destinationPort:  destinationPort,
		logger:           logging.DiscardLogger,
	}

	for _, opt := range opts {
		opt(t2t)
	}

	return t2t
}

// Close closes the proxy.
func (t2t *TCP2TLS) Close() {
	t2t.closeStartedChan()
}

// ListenAndServe starts the proxy.
func (t2t *TCP2TLS) ListenAndServe(ctx context.Context) error {
	proxyListener, err := t2t.listen()
	if err != nil {
		t2t.closeStartedChan()
		return fmt.Errorf("proxy listener failed to start: %w", err)
	}

	// The documentation for net.TCPListener states that Addr will always return a *net.TCPAddr
	t2t.listeningAddress = proxyListener.Addr().(*net.TCPAddr)
	t2t.logger.InfoContext(ctx, "Proxy is listening", "localAddress", t2t.listeningAddress.String())
	t2t.closeStartedChan()

	// Track incoming connectionsInProgress to ensure that they are all completed prior to returning (and cleanup)
	var connectionsInProgress sync.WaitGroup

	// Close the listener when the context is cancelled
	listenerCloseErr := make(chan error)
	go func() {
		defer close(listenerCloseErr)
		<-ctx.Done()

		connectionsInProgress.Wait() // Don't close the socket until all active connections are complete
		listenerCloseErr <- proxyListener.Close()
	}()

	// Accept and proxy connections
	for {
		clientConnection, err := proxyListener.AcceptTCP()
		if err != nil {
			// Ignore errors caused by context cancellation, and stop accepting new connections
			if t2t.isServeStopping(ctx) {
				break
			}

			t2t.logger.WarnContext(ctx, "Failed to accept client connection", "error", err.Error())
			continue
		}
		t2t.logger.DebugContext(ctx, "Accepted connection", "clientAddress", clientConnection.RemoteAddr().String())

		connectionsInProgress.Go(func() {
			if err := t2t.proxyConnection(ctx, clientConnection); err != nil {
				t2t.logger.WarnContext(ctx, "An error occurred while handling proxy connection",
					"clientAddress", clientConnection.RemoteAddr().String(),
					"destinationHost", t2t.destinationHost,
					"destinationPort", t2t.destinationPort,
					"error", err.Error(),
				)
			}
		})
	}

	// This will block until all connections in progress are completed
	if err := <-listenerCloseErr; err != nil {
		return fmt.Errorf("failed to close proxy listener (socket leak): %w", err)
	}
	return nil
}

func (t2t *TCP2TLS) getTLSConfig(ctx context.Context) (*tls.Config, error) {
	config := &tls.Config{
		ServerName: t2t.destinationHost,
	}

	if t2t.clientCertProvider != nil {
		cert, err := t2t.clientCertProvider.GetClientCertificate(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get client certificate from provider: %w", err)
		}

		config.Certificates = append(config.Certificates, *cert)
	}

	return config, nil
}

// listen creates a new localhost TCP socket to listen for incoming connections.
func (t2t *TCP2TLS) listen() (*net.TCPListener, error) {
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("127.0.0.1", "0"))
	if err != nil {
		return nil, fmt.Errorf("failed to bind to a free TCP port on loopback address: %w", err)
	}

	proxyListener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen for TCP connections on %q: %w", addr.AddrPort().String(), err)
	}

	return proxyListener, nil
}

func (t2t *TCP2TLS) proxyConnection(ctx context.Context, clientConnection *net.TCPConn) (err error) {
	defer func() {
		closeErr := clientConnection.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("failed to close client connection (connection leak): %w", closeErr)
		}
		err = errors.Join(err, closeErr)
	}()

	// Build a new TLS dialer
	config, err := t2t.getTLSConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to build TLS config: %w", err)
	}

	destinationDialer := &tls.Dialer{
		Config: config,
	}

	// Establish a new TLS connection to the reverse proxy. Don't share connections to avoid
	// connection sharing bugs
	destinationAddr := net.JoinHostPort(t2t.destinationHost, t2t.destinationPort)
	destinationConnection, err := destinationDialer.DialContext(ctx, "tcp", destinationAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to destination address: %w", err)
	}
	defer func() {
		closeErr := destinationConnection.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("failed to close destination connection (connection leak): %w", closeErr)
		}
		err = errors.Join(err, closeErr)
	}()

	// Docs state that this will always be a `*tls.Conn`
	destinationTLSConnection := destinationConnection.(*tls.Conn)
	deadline, ok := ctx.Deadline()
	if ok {
		if err := destinationTLSConnection.SetDeadline(deadline); err != nil {
			return fmt.Errorf("failed to set TLS connection deadline: %w", err)
		}
	}

	// Read and write until the connection is closed
	// This will not allocate a buffer because the TCP connection implements
	// io.ReadFrom _and_ io.WriteTo.
	readFromDestinationErr := ctxcopy.CopyConcurrently(ctx, destinationTLSConnection, clientConnection)
	writeFromDestinationErr := ctxcopy.CopyConcurrently(ctx, clientConnection, destinationTLSConnection)

	// Wait for all reads and writes to complete
	readErr := <-readFromDestinationErr
	writeErr := <-writeFromDestinationErr

	if !t2t.isServeStopping(ctx) {
		if readErr != nil {
			readErr = fmt.Errorf("failed to read all data from the destination stream")
		}

		if writeErr != nil {
			writeErr = fmt.Errorf("failed to write all data to the destination stream")
		}

		return errors.Join(readErr, writeErr)
	}

	return nil
}

func (t2t *TCP2TLS) isServeStopping(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// Gets the address that the proxy is listening on. This will block until the proxy begins listening,
// the listener errors, or the context is cancelled.
func (t2t *TCP2TLS) GetAddress(ctx context.Context) (*net.TCPAddr, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t2t.started:
	}

	if t2t.listeningAddress == nil {
		return nil, fmt.Errorf("failed to get listening address (listen failed)")
	}

	return t2t.listeningAddress, nil
}
