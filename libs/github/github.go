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
	"time"

	go_github "github.com/google/go-github/v63/github"
	"golang.org/x/oauth2"
)

const (
	OutputEnv     = "GITHUB_OUTPUT"
	ClientTimeout = 30 * time.Second
)

type Client struct {
	client *go_github.Client
}

// New returns a new GitHub Client.
func New(ctx context.Context, token string) (*Client, error) {
	clt := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))

	clt.Timeout = ClientTimeout

	return &Client{
		client: go_github.NewClient(clt),
	}, nil
}
