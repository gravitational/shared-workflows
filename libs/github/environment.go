package github

import (
	"context"
	"net/url"
)

type GetEnvironmentInfo struct {
	Org         string
	Repo        string
	Environment string
}

type Environment struct {
	ID   int64
	Name string
	Org  string
	Repo string
}

func (c *Client) GetEnvironment(ctx context.Context, org, repo string, environment string) (Environment, error) {
	obj, _, err := c.client.Repositories.GetEnvironment(ctx, org, repo, url.PathEscape(environment))
	if err != nil {
		return Environment{}, err
	}

	return Environment{
		ID:   obj.GetID(),
		Name: obj.GetName(),
		Org:  org,
		Repo: repo,
	}, nil
}
