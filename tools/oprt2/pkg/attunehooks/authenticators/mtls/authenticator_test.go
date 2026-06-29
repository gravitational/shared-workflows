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
	"testing"

	"github.com/stretchr/testify/assert"
)

// func TestNewAuthenticator(t *testing.T) {
// 	tests := []struct {
// 		name string
// 	}{}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {

// 		})
// 	}
// }

func TestSetHostPort(t *testing.T) {
	tests := []struct {
		name           string
		attuneEndpoint string
		expectedHost   string
		expectedPort   string
		errFunc        assert.ErrorAssertionFunc
	}{
		{
			name:           "with URL",
			attuneEndpoint: "https://attune.endpoint",
			expectedHost:   "attune.endpoint",
			expectedPort:   "443",
		},
		{
			name:           "with URL and port",
			attuneEndpoint: "https://attune.endpoint:123",
			expectedHost:   "attune.endpoint",
			expectedPort:   "123",
		},
		{
			name:           "with URL, port, path",
			attuneEndpoint: "https://attune.endpoint:123/some/path",
			expectedHost:   "attune.endpoint",
			expectedPort:   "123",
		},
		{
			name:           "with HTTP",
			attuneEndpoint: "http://attune.endpoint",
			expectedHost:   "attune.endpoint",
			expectedPort:   "80",
		},
		{
			name:           "malformed URL",
			attuneEndpoint: "https://invalid domain:123/some/path",
			errFunc:        assert.Error,
		},
		{
			name:           "with host:port",
			attuneEndpoint: "attune.endpoint:456",
			expectedHost:   "attune.endpoint",
			expectedPort:   "456",
		},
		{
			name:    "with nothing",
			errFunc: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errFunc == nil {
				tt.errFunc = assert.NoError
			}

			authenticator := &Authenticator{}

			err := authenticator.setHostPort(tt.attuneEndpoint)

			tt.errFunc(t, err)
			assert.Equal(t, tt.expectedHost, authenticator.attuneEndpointHost)
			assert.Equal(t, tt.expectedPort, authenticator.attuneEndpointPort)
		})
	}
}

func TestSetup(t *testing.T) {
	tests := []struct {
		name string
	}{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authenticator := &Authenticator{
				attuneEndpointHost: "remote-host",
				attuneEndpointPort: "remote-port",
			}
		})
	}
}
