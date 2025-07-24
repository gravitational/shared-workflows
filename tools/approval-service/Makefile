TOOL_NAME = approval-service
PACKAGE_PATH = ./cmd/approval-service.go
VERSION = v0.0.1


include ../repo-release-tooling/tooling.mk

# Override the lint command to use a specific version of golangci-lint
# The global version has some erroneous errors that are not present in later versions.
lint:
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.3.0 run ./... --output.text.colors
