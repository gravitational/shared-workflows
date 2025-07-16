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

package eventsources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// Server receives events from build systems.
// It's typical for build systems to send events via webhooks.
// This consolidates webhook events into a single server with a single endpoint.
type Server struct {
	addr     string       // Address to listen on (ignored if ln is set)
	ln       net.Listener // Custom listener; if set, addr is ignored
	handlers map[string]http.Handler

	mux *http.ServeMux

	log *slog.Logger
}

// ServerOpt is a functional option for the server.
type ServerOpt func(*Server) error

// WithAddress sets the address the server will listen on.
func WithAddress(addr string) ServerOpt {
	return func(s *Server) error {
		if addr == "" {
			return errors.New("address cannot be empty")
		}
		s.addr = addr
		return nil
	}
}

// WithHandler registers a [http.Handler] for a specific path.
// The pattern follows the same rules defined for [http.ServeMux].
// This function has some logic to avoid panics that may occur when registering handlers such as:
// - if the pattern is empty, return an error.
// - if the handler is nil, return an error.
// - if the pattern is already registered by another handler, return an error.
func WithHandler(pattern string, handler http.Handler) ServerOpt {
	return func(s *Server) error {
		if pattern == "" {
			return errors.New("path cannot be empty")
		}
		if handler == nil {
			return errors.New("handler cannot be nil")
		}

		// Check if the path already exists in the handlers map.
		if _, ok := s.handlers[pattern]; ok {
			return fmt.Errorf("handler for path %q already exists", pattern)
		}
		s.handlers[pattern] = handler

		return nil
	}
}

// WithLogger sets the logger for the server.
func WithLogger(logger *slog.Logger) ServerOpt {
	return func(s *Server) error {
		s.log = logger
		return nil
	}
}

func NewServer(opt ...ServerOpt) (*Server, error) {
	s := &Server{
		log:      slog.Default(),
		handlers: make(map[string]http.Handler),
	}
	for _, o := range opt {
		if err := o(s); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	if s.addr == "" && s.ln == nil {
		return nil, errors.New("address or listener must be set")
	}

	if len(s.handlers) == 0 {
		return nil, errors.New("no handlers registered: at least one handler must be registered before starting the server")
	}

	return s, nil
}

// Setup sets up the server.
// This includes starting the listener and setting up any necessary routes.
// If a custom listener (s.ln) is set, the address (s.addr) is ignored.
func (s *Server) Setup(ctx context.Context) error {
	if s.ln == nil {
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			return err
		}
		s.ln = ln
	}

	s.mux = http.NewServeMux()

	for path, handler := range s.handlers {
		s.mux.Handle(path, handler)
	}

	return nil
}

// Run starts the server.
// This blocks until the context is stopped or a fatal error occurs.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Handler: s.mux,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		// When this returns, the errgroup context will be canceled, which will trigger the shutdown process.
		return srv.Serve(s.ln)
	})

	// Listen for context cancellation to gracefully shut down the server.
	eg.Go(func() error {
		<-ctx.Done()
		err := ctx.Err()
		// Create a new context with a timeout for the shutdown process.
		// It's likely that if it does take too long, the OS will forcefully terminate the process anyway.
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelShutdown()
		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			// Join the context error and shutdown error to ensure both are reported,
			// as both may provide useful information for debugging shutdown issues.
			err = errors.Join(err, fmt.Errorf("shutting down server: %w", shutdownErr))
		}
		return err
	})

	return eg.Wait()
}
