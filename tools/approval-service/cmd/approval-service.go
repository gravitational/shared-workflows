package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice"
	"github.com/gravitational/shared-workflows/tools/approval-service/internal/approvalservice/githubevents"
)

var logger = slog.Default()

var cfg = approvalservice.Config{
	GitHubEvents: githubevents.Config{
		Address: "127.0.0.1:8080",
		ValidRepos: []string{
			"gravitational/teleport",
		},
		ValidEnvironments: []string{
			"build/stage",
			"publish/stage",
		},
		ValidOrgs: []string{
			"gravitational",
		},
	},
	Teleport: approvalservice.TeleportConfig{
		ProxyAddrs: []string{
			"localhost:3080",
		},
		IdentityFile:  os.Getenv("TELEPORT_IDENTITY_FILE"),
		User:          "bot-approval-service",
		RoleToRequest: "gha-build-prod",
	},
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
}
