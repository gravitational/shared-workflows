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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
)

// CLI a kong struct that acts as the entry point for the Approval Service CLI.
type CLI struct {
	// Start is the command to start the approval service.
	Start StartCmd `cmd:"" help:"Start the approval service."`
}

// StartCmd is a kong struct that contains flags and methods for starting the approval service.
// It allows configuration of the service via a YAML file and optional environment variables for GitHub App authentication.
type StartCmd struct {
	// ConfigFilePath is the path to the configuration file.
	ConfigFilePath string `name:"config" short:"c" type:"existingfile" env:"APPROVAL_SERVICE_CONFIG_FILE" default:"/etc/approval-service/config.yaml" help:"Path to the configuration file."`

	// WebhookSecret is the secret used to verify webhook requests. This is used to ensure that the requests are coming from a trusted source.
	WebhookSecret string `name:"webhook-secret" env:"APPROVAL_SERVICE_WEBHOOK_SECRET" help:"Secret for verifying webhook requests. Overrides the value in the config file."`
	// AppID is the ID of the GitHub App used for webhook events.
	AppID string `name:"app-id" env:"APPROVAL_SERVICE_APP_ID" help:"ID of the GitHub App used for webhook events. Overrides the value in the config file."`
	// InstallationID is the ID of the GitHub App installation.
	InstallationID string `name:"installation-id" env:"APPROVAL_SERVICE_INSTALLATION_ID" help:"Installation ID of the GitHub App used for webhook events. Overrides the value in the config file."`
	// PrivateKeyPath is the path to the private key file for the GitHub App. This is used to authenticate the GitHub App with the GitHub API.
	PrivateKeyPath string `name:"private-key-path" env:"APPROVAL_SERVICE_PRIVATE_KEY_PATH" type:"existingfile" help:"Path to the private key file for the GitHub App. Overrides the value in the config file."`
}

func main() {
	var cli CLI
	kctx := kong.Parse(&cli)
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}

func (cmd *StartCmd) Run() error {
	cfg, err := cmd.loadConfig(cmd.ConfigFilePath)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	svc, err := approvalservice.NewFromConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("initializing approval service: %w", err)
	}

	if err := svc.Setup(ctx); err != nil {
		return fmt.Errorf("setting up approval service: %w", err)
	}

	if err := svc.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("stopping approval service: %w", err)
	}
	return nil
}

func (cmd *StartCmd) loadConfig(path string) (cfg config.Root, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshalling config file %q: %w", path, err)
	}

	if cmd.WebhookSecret != "" {
		cfg.EventSources.GitHub.Secret = cmd.WebhookSecret
	}

	if cmd.AppID != "" {
		appIDInt, err := strconv.ParseInt(cmd.AppID, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("parsing app ID: %w", err)
		}
		cfg.EventSources.GitHub.Authentication.App.AppID = appIDInt
	}

	if cmd.InstallationID != "" {
		installationIDInt, err := strconv.ParseInt(cmd.InstallationID, 10, 64)
		if err != nil {
			return cfg, fmt.Errorf("parsing installation ID: %w", err)
		}
		cfg.EventSources.GitHub.Authentication.App.InstallationID = installationIDInt
	}

	if cmd.PrivateKeyPath != "" {
		privateKeyData, err := os.ReadFile(cmd.PrivateKeyPath)
		if err != nil {
			return cfg, fmt.Errorf("reading private key file %q: %w", cmd.PrivateKeyPath, err)
		}
		cfg.EventSources.GitHub.Authentication.App.PrivateKey = string(privateKeyData)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}
