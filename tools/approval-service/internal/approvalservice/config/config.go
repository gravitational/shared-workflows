package config

// Root is the root configuration for the approval service.
type Root struct {
	// ApprovalService is the configuration for the Pipeline Approval Service
	ApprovalService ApprovalService `yaml:"approval_service,omitempty"`

	// GitHubApp is the configuration for the GitHub App.
	EventSources EventSources `yaml:"event_sources,omitempty"`
}

// ApprovalService contains configuration related to the approval service.
type ApprovalService struct {
	// Teleport is the configuration for the Teleport client.
	Teleport Teleport `yaml:"teleport,omitempty"`
}

type EventSources struct {
	// GitHub is the configuration for the GitHub App and webhook.
	// This is used to listen for events from GitHub and to respond to approvals.
	GitHub []GitHubSource `yaml:"github,omitempty"`
}

// GitHubEvents represents the per-repo configuration for webhook events and API authentication.
type GitHubSource struct {
	// GitHubWebhookAddr should be the full URL to listen to webhooks events from.
	// For example: 127.0.0.1:8080/webhook
	WebhookAddr string `yaml:"webhook_addr,omitempty"`
	// Org is the organization that the event must be from.
	Org string `yaml:"org,omitempty"`
	// Repo is the repository that the event must be from.
	Repo string `yaml:"repo,omitempty"`
	// Environments is a list of environments that the event must be for.
	Environments []string `yaml:"environments,omitempty"`

	// The following are credentials for authentication.
	Secret         string `yaml:"secret,omitempty"`
	AppID          int64  `yaml:"app_id"`
	InstallationID int64  `yaml:"installation_id"`
	PrivateKeyPath string `yaml:"private_key_path"`
}

type Teleport struct {
	// ProxyAddrs is a list of Teleport proxy addresses to connect to.
	ProxyAddrs []string `yaml:"proxy_addrs"`
	// IdentityFile is the path to the Teleport identity file.
	IdentityFile string `yaml:"identity_file"`
	// User is the Teleport user to use for creating the access request.
	User string `yaml:"user"`
	// RoleToRequest is the Teleport role to request for the access request.
	RoleToRequest string `yaml:"role_to_request"`
	// RequestTTLHours is used to determine the expiry.
	// By default this is 7*24 hours (7 days).
	RequestTTLHours int64 `yaml:"request_ttl_hours"`
}
