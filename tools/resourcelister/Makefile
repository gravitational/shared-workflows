.PHONY: build test clean run install help

# Default target
.DEFAULT_GOAL := help

# Binary name
BINARY_NAME=resourcelister

# Build the application
build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) .
	@echo "Build complete: ./$(BINARY_NAME)"

# Run tests
test: ## Run all tests
	@echo "Running tests..."
	@go test ./... -v

# Run tests with coverage
coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test ./... -cover -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean: ## Remove binary and build artifacts
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f coverage.out coverage.html
	@rm -f *.json
	@echo "Clean complete"

# Run the application (requires AWS credentials)
run: ## Run the application
	@go run main.go

# Install dependencies
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...

# Run linters
lint: ## Run linters (requires golangci-lint)
	@echo "Running linters..."
	@golangci-lint run

# Build for multiple platforms
build-all: ## Build for multiple platforms
	@echo "Building for multiple platforms..."
	@GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 .
	@GOOS=linux GOARCH=arm64 go build -o $(BINARY_NAME)-linux-arm64 .
	@GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 .
	@GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 .
	@GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe .
	@echo "Multi-platform build complete"

# Install the binary to $GOPATH/bin
install: build ## Install the binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@go install .
	@echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

# Help command
help: ## Show this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
