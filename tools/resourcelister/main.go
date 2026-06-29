package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/resourceexplorer2"
	"github.com/shared-workflows/tools/resourcelister/lister"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := resourceexplorer2.NewFromConfig(cfg)
	l := lister.NewLister(client)

	resources, err := l.ListAllResources(ctx)
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	// encoder.SetIndent("", "  ")
	if err := encoder.Encode(resources); err != nil {
		return fmt.Errorf("failed to encode resources to JSON: %w", err)
	}

	return nil
}
