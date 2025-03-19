package github

import "context"

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

func (c *Client) GetEnvironment(ctx context.Context, info GetEnvironmentInfo) (Environment, error) {
	env := Environment{}
	obj, _, err := c.client.Repositories.GetEnvironment(ctx, info.Org, info.Repo, info.Environment)
	if err != nil {
		return env, err
	}

	env.ID = obj.GetID()
	env.Name = obj.GetName()
	env.Org = info.Org
	env.Repo = info.Repo

	return env, nil
}
