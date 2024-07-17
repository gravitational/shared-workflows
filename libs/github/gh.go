/*
Copyright 2024 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package github

import (
	"context"
	"errors"

	"github.com/cli/go-gh/v2/pkg/auth"
)

var ErrTokenNotFound = errors.New("could not find a GitHub token configured on system")

// NewClientFromGHAuth will use the gh credential chain to initialize the client.
// Useful for initialization both in CI and in user environments.
// Will check in order: GITHUB_TOKEN env var, gh config file, gh system keyring (gh auth login).
func NewClientFromGHAuth(ctx context.Context) (*Client, error) {
	token, _ := auth.TokenForHost("github.com")
	if token == "" {
		return &Client{}, ErrTokenNotFound
	}

	return New(ctx, token)
}
