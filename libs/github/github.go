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
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// NewForApp returns a new GitHub Client with authentication for a GitHub App.
func NewForApp(ctx context.Context, appID int64, installationID int64, privateKey []byte) (*Client, error) {
	appTr, err := newAppTransport(ctx, appID, installationID, privateKey)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: appTr}
	httpClient.Timeout = ClientTimeout

	cl := go_github.NewClient(httpClient)
	return &Client{
		client: cl,
		search: cl.Search,
	}, nil
}

// installationAuthTransport is a middleware that adds GitHub App authentication to HTTP requests.
// It implements the http.RoundTripper interface, allowing it to be used as a transport for the underlying HTTP client passed to the GitHub client.
//
// For more details: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app
type installationAuthTransport struct {
	// tr is the underlying http.RoundTripper that will be used to make requests.
	tr http.RoundTripper

	appsClient     *go_github.Client // GitHub client for making API requests to the GitHub Apps API
	installationID int64

	token *go_github.InstallationToken
	mu    sync.Mutex // mutex to protect access to the token
}

// jwtAuthTransport is a middleware that adds JWT authentication to HTTP requests.
// It implements the http.RoundTripper interface, allowing it to be used as a transport for the underlying HTTP client passed to the GitHub client.
// This transport is useful for making certain GitHub API requests that require a JWT token, such as creating an installation access token for a GitHub App.
//
// For more details: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-json-web-token-jwt-for-a-github-app
type jwtAuthTransport struct {
	// tr is the underlying http.RoundTripper that will be used to make requests.
	tr http.RoundTripper

	appID      int64
	privateKey *rsa.PrivateKey
}

// newJWTClient creates a new GitHub client that uses JWT authentication.
// This client is typically used for operations that require a JWT token, such as creating an installation access token for a GitHub App.
func newJWTClient(appID int64, privateKey []byte) (*go_github.Client, error) {
	// Parse the private key from the provided byte slice.
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing rsa private key: %w", err)
	}
	return go_github.NewClient(&http.Client{
		Transport: &jwtAuthTransport{
			tr:         http.DefaultTransport,
			appID:      appID,
			privateKey: privKey,
		},
	}), nil
}

func newAppTransport(ctx context.Context, appID, installationID int64, privateKey []byte) (*installationAuthTransport, error) {
	appsClient, err := newJWTClient(appID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("creating JWT client: %w", err)
	}

	tr := &installationAuthTransport{
		installationID: installationID,
		tr:             http.DefaultTransport,
		appsClient:     appsClient,
	}

	_, err = tr.getAccessToken(ctx) // Pre-fetch the access token to ensure the transport is ready for use.
	if err != nil {
		return nil, fmt.Errorf("pre-fetching access token: %w", err)
	}

	return tr, nil
}

// RoundTrip implements the http.RoundTripper interface for the jwtTransport.
func (j *jwtAuthTransport) RoundTrip(orig *http.Request) (*http.Response, error) {
	if orig.Body != nil {
		// As per the http.RoundTripper contract, we should close the body after we're done with it.
		defer orig.Body.Close()
	}
	req := orig.Clone(orig.Context()) // clone the request to avoid modifying the original

	// Account for clock skew by setting the issued at time to 60 seconds in the past.
	iss := time.Now().Add(-60 * time.Second)
	exp := iss.Add(2 * time.Minute)
	claims := &jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(iss),
		ExpiresAt: jwt.NewNumericDate(exp),
		Issuer:    strconv.FormatInt(j.appID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	ss, err := token.SignedString(j.privateKey)
	if err != nil {
		return nil, fmt.Errorf("signing jwt: %s", err)
	}

	req.Header.Set("Authorization", "Bearer "+ss)

	return j.tr.RoundTrip(req)
}

// RoundTrip implements the http.RoundTripper interface for the appTransport.
func (a *installationAuthTransport) RoundTrip(orig *http.Request) (*http.Response, error) {
	if orig.Body != nil {
		// As per the http.RoundTripper contract, we should close the body after we're done with it.
		defer orig.Body.Close()
	}
	req := orig.Clone(orig.Context()) // clone the request to avoid modifying the original

	// Get the access token for the installation.
	token, err := a.getAccessToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("getting access token: %w", err)
	}
	// Set the Authorization header with the installation access token.
	req.Header.Set("Authorization", "Bearer "+token)

	// Add GitHub App specific headers or logic here if needed
	return a.tr.RoundTrip(req)
}

func (a *installationAuthTransport) getAccessToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If the token is nil or expired, create a new one.
	if a.token == nil || a.token.GetToken() == "" || time.Now().After(a.token.GetExpiresAt().Time) {
		newToken, err := a.createAccessToken(ctx)
		if err != nil {
			return "", fmt.Errorf("creating access token: %w", err)
		}
		a.token = newToken
	}

	return a.token.GetToken(), nil
}

func (a *installationAuthTransport) createAccessToken(ctx context.Context) (*go_github.InstallationToken, error) {
	token, _, err := a.appsClient.Apps.CreateInstallationToken(ctx, a.installationID, &go_github.InstallationTokenOptions{})
	if err != nil {
		return nil, fmt.Errorf("calling creating installation token API: %w", err)
	}
	return token, nil
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
