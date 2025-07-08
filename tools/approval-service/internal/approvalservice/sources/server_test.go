package sources

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Run("Handler Registration", func(t *testing.T) {
		tests := map[string]struct {
			opts        []ServerOpt
			expectError bool
		}{
			"valid options": {
				opts: []ServerOpt{
					WithAddress(":8080"),
					WithHandler("/path", okHandler),
				},
				expectError: false,
			},
			"invalid address": {
				opts: []ServerOpt{
					WithAddress(""),
				},
				expectError: true,
			},
			"path conflicts": {
				opts: []ServerOpt{
					WithAddress(":8080"),
					WithHandler("/path", okHandler),
					WithHandler("/path", okHandler),
				},
				expectError: true,
			},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				_, err := NewServer(tt.opts...)
				if tt.expectError {
					assert.Error(t, err)
					return
				}

				assert.NoError(t, err)
			})
		}
	})

	t.Run("Handlers Receive Requests", func(t *testing.T) {
		ln, err := net.Listen("tcp", ":0")
		require.NoError(t, err)
		t.Cleanup(func() {
			err = ln.Close()
			assert.NoError(t, err)
		})

		s, err := NewServer(
			withListener(ln),
			WithHandler("/health", okHandler),
			WithHandler("POST /test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, "test request", string(body))
				w.WriteHeader(http.StatusOK)
			})),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		err = s.Setup(ctx)
		require.NoError(t, err)

		go func() {
			err := s.Run(ctx)
			assert.NoError(t, err)
		}()

		reqUrl, err := url.JoinPath("http://"+ln.Addr().String(), "/health")
		resp, err := http.Get(reqUrl)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		reqUrl, err = url.JoinPath("http://"+ln.Addr().String(), "/test")
		require.NoError(t, err)
		resp, err = http.Post(reqUrl, "text/plain", strings.NewReader("test request"))
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func withListener(ln net.Listener) ServerOpt {
	return func(s *Server) error {
		s.ln = ln
		return nil
	}
}
