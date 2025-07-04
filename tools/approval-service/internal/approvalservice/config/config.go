package config

import (
	"errors"
	"fmt"
	"strings"
)

// Root is the root configuration for the approval service.
type Root struct {
	// ApprovalService is the configuration for the Pipeline Approval Service
	ApprovalService ApprovalService `yaml:"approval_service,omitempty"`

	// EventSources is the configuration for the event sources.
	EventSources EventSources `yaml:"event_sources,omitempty"`
}

// ApprovalService contains configuration related to the approval service.
type ApprovalService struct {
	// Teleport is the configuration for the Teleport client.
	Teleport Teleport `yaml:"teleport,omitempty"`
	// Address is the address the approval service will listen for events on
	Address string `yaml:"address,omitempty"`
}

type EventSources struct {
	// GitHub is the configuration for the GitHub App and webhook.
	// This is used to listen for events from GitHub and to respond to approvals.
	GitHub []GitHubSource `yaml:"github,omitempty"`
}

// GitHubSource represents the per-repo configuration for webhook events and API authentication.
type GitHubSource struct {
	Path string `yaml:"path,omitempty"`
	// Org is the organization that the event must be from.
	Org string `yaml:"org,omitempty"`
	// Repo is the repository that the event must be from.
	Repo string `yaml:"repo,omitempty"`
	// Environments is a list of environments that the event must be for.
	Environments []string `yaml:"environments,omitempty"`
	// Secret is the secret token used to verify the webhook events.
	Secret string `yaml:"secret,omitempty"`
	// Authentication configuration for the GitHub REST API
	Authentication GitHubAuthentication `yaml:"authentication,omitempty"`
}

// GitHubAuthentication is the configuration for the GitHub App authentication.
type GitHubAuthentication struct {
	// App is the configuration for the GitHub App authentication.
	App GitHubAppAuthentication `yaml:"app,omitempty"`
}

// GitHubAppAuthentication is the configuration for the GitHub App authentication.
type GitHubAppAuthentication struct {
	// AppID is the ID of the GitHub App.
	AppID int64 `yaml:"app_id"`
	// InstallationID is the ID of the GitHub App installation.
	InstallationID int64 `yaml:"installation_id"`
	// PrivateKeyPath is the path to the private key for the GitHub App.
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

func (r *Root) Validate() error {
	if err := r.ApprovalService.Validate(); err != nil {
		return fmt.Errorf("approval service: %w", err)
	}
	if err := r.EventSources.Validate(); err != nil {
		return fmt.Errorf("event sources: %w", err)
	}
	return nil
}

func (c *ApprovalService) Validate() error {
	return c.Teleport.Validate()
}

func (c *EventSources) Validate() error {
	if len(c.GitHub) == 0 {
		return errors.New("at least one event source is required")
	}
	for _, gh := range c.GitHub {
		if err := gh.Validate(); err != nil {
			return fmt.Errorf("github: %w", err)
		}
	}
	return nil
}

func (c *GitHubSource) Validate() error {
	missing := []string{}
	if c.Path == "" {
		missing = append(missing, "path")
	}
	if c.Org == "" {
		missing = append(missing, "org")
	}
	if c.Repo == "" {
		missing = append(missing, "repo")
	}
	if len(c.Environments) == 0 {
		missing = append(missing, "environments")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *GitHubAuthentication) Validate() error {
	if c.App == (GitHubAppAuthentication{}) {
		return errors.New("github app authentication is required")
	}
	if err := c.App.Validate(); err != nil {
		return fmt.Errorf("github app: %w", err)
	}
	return nil
}

func (c *GitHubAppAuthentication) Validate() error {
	missing := []string{}
	if c.AppID == 0 {
		missing = append(missing, "app_id")
	}
	if c.InstallationID == 0 {
		missing = append(missing, "installation_id")
	}
	if c.PrivateKeyPath == "" {
		missing = append(missing, "private_key_path")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *Teleport) Validate() error {
	missing := []string{}
	if len(c.ProxyAddrs) == 0 {
		missing = append(missing, "proxy_addrs")
	}
	if c.IdentityFile == "" {
		missing = append(missing, "identity_file")
	}
	if c.User == "" {
		missing = append(missing, "user")
	}
	if c.RoleToRequest == "" {
		missing = append(missing, "role_to_request")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
