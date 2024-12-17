OS ?= $(shell uname -s | tr '[[:upper:]]' '[[:lower:]]')
ARCH ?= $(shell uname -m)
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
	@golangci-lint run ./... --out-format colored-line-number -vvv

test:
	@gotestsum --format github-actions ./... -- -count 100 -shuffle on -timeout 2m -race

binary:
	@echo "Building for $(OS)/$(ARCH) and writing to $(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)"
	@GOOS=$(OS) GOARCH=$(ARCH) go build -o "$(BUILD_DIR)/" -ldflags="-s -w" "$(PACKAGE_PATH)"

tarball: TARBALL_NAME = $(TOOL_NAME)-$(VERSION)-$(OS)-$(ARCH).tar.gz
tarball: binary
	@tar -C "$(BUILD_DIR)" -czf "$(BUILD_DIR)/$(TARBALL_NAME)" "$(BINARY_NAME)"

container-image: OS = linux
container-image: binary
	@docker buildx build --platform="linux/$(ARCH)" -t "$(TOOL_NAME):$(CONTAINER_VERSION)" \
		--build-arg "FILE_NAME=$(BINARY_NAME)" --file "$(DOCKERFILE_PATH)" .

clean:
	@rm -rf build/
	@docker image rm -f "$(TOOL_NAME):$(CONTAINER_VERSION)" 2> /dev/null > /dev/null

.PHONY: print-tool-name print-version lint test binary tarball container-image clean
