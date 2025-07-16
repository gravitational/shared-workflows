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
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/config"
)

type CLI struct {
	ConfigFilePath string `name:"config" short:"c" type:"existingfile" env:"APPROVAL_SERVICE_CONFIG_FILE" default:"/etc/approval-service/config.yaml" help:"Path to the configuration file."`
}

func main() {
	var cli CLI
	kctx := kong.Parse(&cli)
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}

func (cli *CLI) Run() error {
	cfg, err := parseConfig(cli.ConfigFilePath)
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

func parseConfig(path string) (cfg config.Root, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshalling config file %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}
