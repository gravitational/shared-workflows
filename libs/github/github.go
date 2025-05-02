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
	"fmt"
	"io"
	"time"

	go_github "github.com/google/go-github/v71/github"
	"golang.org/x/oauth2"
)

const (
	OutputEnv     = "GITHUB_OUTPUT"
	ClientTimeout = 30 * time.Second
)

type Client struct {
	client *go_github.Client
	search searchService
}

type searchService interface {
	Issues(ctx context.Context, query string, opts *go_github.SearchOptions) (*go_github.IssuesSearchResult, *go_github.Response, error)
}

// New returns a new GitHub Client.
func New(ctx context.Context, token string) (*Client, error) {
	clt := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))

	clt.Timeout = ClientTimeout
	cl := go_github.NewClient(clt)
	return &Client{
		client: cl,
		search: cl.Search,
	}, nil
}

// Sometimes the error can be eaten by the underlying client library.
// It seems that in some circumstances we can get an inconsistent body structure and the underlying client library won't parse it correctly.
// If this is the case for a function, this is a workaround to just get the raw body as an error message.
func errorFromBody(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if len(data) == 0 {
		return nil
	}

	return fmt.Errorf("unexpected response %q", data)
}

// NewForApp returns a new GitHub Client with authentication for a GitHub App.
func NewForApp(appID int64, installationID int64, privateKey []byte) (*Client, error) {
	itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, privateKey)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: itr}
	httpClient.Timeout = ClientTimeout

	cl := go_github.NewClient(httpClient)
	return &Client{
		client: cl,
		search: cl.Search,
	}, nil
}
