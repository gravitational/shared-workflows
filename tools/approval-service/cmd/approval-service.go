package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/config"
	"github.com/stretchr/testify/assert/yaml"
)

type CLI struct {
	Config string `type:"path"`
}

// Process:
// 1. Take in events from CI/CD systems
// 2. Extract common information
// 3. Process information according to business rules/logic
// 4. Callback to the event source, have it handle

// One of the design goals of this is to support multiple "sources" of deployment events,
// such as github or another CI/CD service.

// Skeleton TODO:
// * add ctx where needed
// * err handling
// * pass some form of "config" struct to setup funcs, which will be populated by CLI or config file
// * maybe add some "hook" for registering CLI options?
// * Move approval processor, event, and event source to different packages

func main() {
	var cli CLI
	kctx := kong.Parse(&cli)
	err := kctx.Run()
	kctx.FatalIfErrorf(err)
}

func (cli *CLI) Run() error {
	cfg, err := parseConfig(cli.Config)
	if err != nil {
		log.Fatal(err)
	}

	svc, err := approvalservice.NewApprovalService(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go func() {
		errc <- svc.Run(ctx)
		close(errc)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigs
		log.Println("Caught signal, shutting down...")
		cancel()
	}()

	select {
	case err := <-errc:
		log.Fatal(err)
	case <-ctx.Done():
		log.Println("Gracefully shutting down...")
	}

	return nil
}

func parseConfig(path string) (cfg config.Root, err error) {
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer func() {
		cerr := f.Close()
		if err == nil {
			err = cerr
		}
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
