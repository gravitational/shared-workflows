.PHONY: build test clean run install help coverage deps fmt lint build-all

.DEFAULT_GOAL := help

BINARY_NAME=amicleanup

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "Build complete: ./$(BINARY_NAME)"

test: ## Run all tests
	@echo "Running tests..."
	@go test ./... -race -v

coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test ./... -cover -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

clean: ## Remove binary and build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

run: ## Run the application (uses --action flag at minimum)
	@go run main.go $(ARGS)

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

lint: ## Run linters (requires golangci-lint)
	@echo "Running linters..."
	@golangci-lint run

install: build ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@go install .
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
