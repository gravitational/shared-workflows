package config

// Root is the root configuration for the approval service.
type Root struct {
	// GitHubWebhook is the configuration for the GitHub webhook.
	GitHubEvents GitHubEvents `yaml:"github_events,omitempty"`

	// Teleport is the configuration for the Teleport client.
	Teleport Teleport `yaml:"teleport,omitempty"`

	// GitHubApp is the configuration for client authentication as a GitHub App.
	GitHubApp GitHubApp `yaml:"github_app,omitempty"`
}

type GitHubEvents struct {
	// Address is the address to listen for GitHub webhooks.
	Address string `yaml:"address,omitempty"`
	// Secret is the secret used to authenticate the webhook.
	Secret string `yaml:"secret,omitempty"`
	// Validation is a list of validation configurations that the event must match.
	Validation []Validation `yaml:"validation,omitempty"`
}

// Validation is the configuration for validation checks.
type Validation struct {
	// Org is the organization that the event must be from.
	Org string `yaml:"org,omitempty"`
	// Repo is the repository that the event must be from.
	Repo string `yaml:"repo,omitempty"`
	// Environments is a list of environments that the event must be for.
	Environments []string `yaml:"environments,omitempty"`
}

type Teleport struct {
	ProxyAddrs    []string `yaml:"proxy_addrs"`
	IdentityFile  string   `yaml:"identity_file"`
	User          string   `yaml:"user"`
	RoleToRequest string   `yaml:"role_to_request"`
}

type GitHubApp struct {
	AppID          int64  `yaml:"app_id"`
	InstallationID int64  `yaml:"installation_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
}
