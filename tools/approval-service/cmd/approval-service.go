package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"gopkg.in/yaml.v2"
)

type CLI struct {
	ConfigFilePath string `name:"config" short:"c" type:"existingfile" env:"PAS_CONFIG_FILE" default:"/etc/approval-service/config.yaml" help:"Path to the configuration file."`
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
		return err
	}

	svc, err := approvalservice.NewApprovalService(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("initializing approval service: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt, syscall.SIGTERM)

	if err := svc.Setup(ctx); err != nil {
		return fmt.Errorf("setting up approval service: %w", err)
	}

	errc := make(chan error)
	go func() {
		defer close(errc)
		defer cancel()
		errc <- svc.Run(ctx)
	}()

	if err := <-errc; err != nil && !errors.Is(err, context.Canceled) {
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
		return cfg, err
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}
