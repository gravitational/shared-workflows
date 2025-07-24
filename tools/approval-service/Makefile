TOOL_NAME = approval-service
PACKAGE_PATH = ./cmd/approval-service.go
VERSION = v0.0.1


include ../repo-release-tooling/tooling.mk

lint:
	@go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.3.0 run ./... --output.text.colors
