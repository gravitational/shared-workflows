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
}

// TeleportConfig is the configuration for the Teleport client.
type TeleportConfig struct {
	ProxyAddrs   []string `json:"proxy_addrs"`
	IdentityFile string   `json:"identity_file"`
}
