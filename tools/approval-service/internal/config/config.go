/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package config

import (
	"fmt"
	"strings"
	"time"
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
	// ListenAddr is the address the approval service will listen for events on
	ListenAddr string `yaml:"listen_addr,omitempty"`
	// ReconcileInterval is the interval at which the service will reconcile waiting workflows.
	// This is used to ensure that workflows that are waiting for approval are processed in a timely manner.
	ReconcileInterval time.Duration `yaml:"reconcile_interval,omitempty"`
}

type EventSources struct {
	// GitHub is the configuration for the GitHub App and webhook.
	// This is used to listen for events from GitHub and to respond to approvals.
	GitHub GitHubSource `yaml:"github,omitempty"`
}

// GitHubSource represents the per-repo configuration for webhook events and API authentication.
type GitHubSource struct {
	// Path is the URL path that the webhook will be sent to.
	Path string `yaml:"path,omitempty"`
	// Org is the organization that the event must be from.
	Org string `yaml:"org,omitempty"`
	// Repo is the repository that the event must be from.
	Repo string `yaml:"repo,omitempty"`
	// Environments is a list of environments that the event must be for.
	Environments []GitHubEnvironment `yaml:"environments,omitempty"`
	// Secret is the secret token used to verify the webhook events.
	Secret string `yaml:"secret,omitempty"`
	// Authentication configuration for the GitHub REST API
	Authentication GitHubAuthentication `yaml:"authentication,omitempty"`
}

// GitHubEnvironment configures the environment and the associated Teleport Role to request for that environment.
type GitHubEnvironment struct {
	// Name is the name of the environment.
	Name string `yaml:"name"`
	// TeleportRole is the Teleport role to request for this environment.
	TeleportRole string `yaml:"teleport_role"`
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
	// PrivateKey is the private key of the GitHub App.
	// Normal usage this is populated from a filepath in the CLI using the `--private-key-path` flag.
	// This can be passed as a base64 encoded string in YAML using `!! binary ${key}` or just as a non-encoded string.
	PrivateKey string `yaml:"private_key"`
}

type Teleport struct {
	// ProxyAddrs is a list of Teleport proxy addresses to connect to.
	ProxyAddrs []string `yaml:"proxy_addrs"`
	// IdentityFile is the path to the Teleport identity file.
	IdentityFile string `yaml:"identity_file"`
	// User is the Teleport user to use for creating the access request.
	User string `yaml:"user"`
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
	return c.GitHub.Validate()
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

	for _, env := range c.Environments {
		if err := env.Validate(); err != nil {
			return fmt.Errorf("environment %s: %w", env.Name, err)
		}
	}

	if c.Secret == "" {
		missing = append(missing, "secret")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *GitHubEnvironment) Validate() error {
	missing := []string{}
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if c.TeleportRole == "" {
		missing = append(missing, "teleport_role")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *GitHubAuthentication) Validate() error {
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

	if len(c.PrivateKey) == 0 {
		missing = append(missing, "private_key")
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

	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}
