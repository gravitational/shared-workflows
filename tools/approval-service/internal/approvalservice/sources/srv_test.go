package sources

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServer(t *testing.T) {
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
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})
