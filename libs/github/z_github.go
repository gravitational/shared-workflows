package github

import (
	"context"
	"time"

	go_github "github.com/google/go-github/v37/github"
	"github.com/shurcooL/githubv4"
	"golang.org/x/oauth2"
)

const (
	OutputEnv     = "GITHUB_OUTPUT"
	ClientTimeout = 30 * time.Second
)

type Client struct {
	client *go_github.Client
	v4api  *githubv4.Client
}

// New returns a new GitHub Client.
func New(ctx context.Context, token string) (*Client, error) {
	tok := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})

	// v3 init
	clt := oauth2.NewClient(ctx, tok)

	// v4 init
	httpClient := oauth2.NewClient(context.Background(), tok)
	v4 := githubv4.NewClient(httpClient)

	clt.Timeout = ClientTimeout

	return &Client{
		client: go_github.NewClient(clt),
		v4api:  v4,
	}, nil
}
