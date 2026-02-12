OS ?= $(shell uname -s | tr '[[:upper:]]' '[[:lower:]]')
ARCH ?= $(shell uname -m | sed 's/aarch64/arm64/')
TOOL_NAME ?= unnamed-dev-tool
BUILD_DIR = build/$(OS)/$(ARCH)
PACKAGE_PATH ?= ./...
VERSION ?= v0.0.0-dev
# Trim the 'v' prefix
CONTAINER_VERSION = $(VERSION:v%=%)
BINARY_NAME = $(TOOL_NAME)
ifeq ($(OS),windows)
BINARY_NAME := $(BINARY_NAME).exe
endif
DOCKERFILE_PATH ?= ../repo-release-tooling/Dockerfile

print-tool-name:
	@echo "$(TOOL_NAME)"

print-version:
	@echo "$(VERSION)"

lint:
	@golangci-lint run ./... -vvv

test:
	@gotestsum $(if $(GITHUB_ACTIONS),--format github-actions) ./... -- -count 100 -shuffle on -timeout 2m -race

generate:
	@go generate ./...

binary: generate
	@echo "Building for $(OS)/$(ARCH) and writing to $(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)"
	@CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o "$(BUILD_DIR)/$(BINARY_NAME)" -ldflags="-s -w" "$(PACKAGE_PATH)"

tarball: TARBALL_NAME = $(TOOL_NAME)-$(VERSION)-$(OS)-$(ARCH).tar.gz
tarball: binary
	@tar -C "$(BUILD_DIR)" -czf "$(BUILD_DIR)/$(TARBALL_NAME)" "$(BINARY_NAME)"

container-image: OS = linux
container-image: binary
	@docker buildx build --platform="linux/$(ARCH)" -t "$(TOOL_NAME):$(CONTAINER_VERSION)" \
		--build-arg "TOOL_NAME=$(BINARY_NAME)" --file "$(DOCKERFILE_PATH)" .

clean:
	@rm -rf build/
	@docker image rm -f "$(TOOL_NAME):$(CONTAINER_VERSION)" 2> /dev/null > /dev/null

.PHONY: print-tool-name print-version lint test generate binary tarball container-image clean
