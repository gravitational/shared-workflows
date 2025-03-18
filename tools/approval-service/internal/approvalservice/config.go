package approvalservice

import (
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
)

// Config is the configuration for the approval service.
type Config struct {
	// GitHubWebhook is the configuration for the GitHub webhook.
	GitHubEvents githubevents.Config `json:"github_events,omitempty"`

	// Teleport is the configuration for the Teleport client.
	Teleport TeleportConfig `json:"teleport,omitempty"`

	// GitHubApp is the configuration for client authentication as a GitHub App.
	GitHubApp GitHubAppConfig `json:"github_app,omitempty"`
}

// TeleportConfig is the configuration for the Teleport client.
type TeleportConfig struct {
	ProxyAddrs    []string `json:"proxy_addrs"`
	IdentityFile  string   `json:"identity_file"`
	User          string   `json:"user"`
	RoleToRequest string   `json:"role_to_request"`
}

type GitHubAppConfig struct {
	AppID          int64  `json:"app_id"`
	InstallationID int64  `json:"installation_id"`
	PrivateKeyPath string `json:"private_key_path"`
}
