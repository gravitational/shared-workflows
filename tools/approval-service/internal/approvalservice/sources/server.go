package sources

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"
)

// Server receives events from build systems.
// It's typical for build systems to send events via webhooks.
// This consolidates webhook events into a single server with a single endpoint.
type Server struct {
	addr     string
	ln       net.Listener
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

// WithListener sets the listener for the server.
// This is an alternative to WithAddress for cases where it's more useful to set the listener directly (e.g. testing).
func WithListener(ln net.Listener) ServerOpt {
	return func(s *Server) error {
		if ln == nil {
			return errors.New("listener cannot be nil")
		}
		s.ln = ln
		return nil
	}
}

func WithHandler(path string, handler http.Handler) ServerOpt {
	return func(s *Server) error {
		if path == "" {
			return errors.New("path cannot be empty")
		}
		if handler == nil {
			return errors.New("handler cannot be nil")
		}
		if s.handlers == nil {
			s.handlers = make(map[string]http.Handler)
		}

		// Check if the path already exists in the handlers map.
		if _, ok := s.handlers[path]; ok {
			return errors.New("handler already exists for path")
		}
		s.handlers[path] = handler

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
		log: slog.Default(),
	}
	for _, o := range opt {
		if err := o(s); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	if s.addr == "" && s.ln == nil {
		return nil, errors.New("address or listener must be set")
	}

	return s, nil
}

// Setup sets up the server.
// This includes starting the listener and setting up any necessary routes.
func (s *Server) Setup(ctx context.Context) error {
	if s.ln == nil {
		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			return err
		}
		s.ln = ln
	}

	s.mux = http.NewServeMux()

	return nil
}

// Run starts the server.
// This blocks until the context is stopped or a fatal error occurs.
func (s *Server) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		<-ctx.Done()
		return s.ln.Close()
	})
	eg.Go(func() error {
		return http.Serve(s.ln, s.mux)
	})
	return eg.Wait()
}
