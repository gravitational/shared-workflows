OS ?= $(shell uname -s | tr '[[:upper:]]' '[[:lower:]]')
ARCH ?= $(shell uname -m)
TOOL_NAME ?= changelog
BUILD_DIR ?= build/$(OS)/$(ARCH)/
VERSION ?= v0.0.0-dev

lint:
	@golangci-lint run ./... --out-format colored-line-number

test:
	@gotestsum --format github-actions ./... -- -shuffle on -timeout 2m -race

binary:
	@echo "Building for $(OS)/$(ARCH) and writing to $(BUILD_DIR)"
	@mkdir -p "$(BUILD_DIR)"
	@GOOS=$(OS) GOARCH=$(ARCH) go build -o "$(BUILD_DIR)/" -ldflags="-s -w"

tarball: TARBALL_NAME = "$(TOOL_NAME)-$(VERSION)-$(OS)-$(ARCH).tar.gz"
tarball: binary
	@tar -C "$(BUILD_DIR)" -czvf "$(BUILD_DIR)/$(TARBALL_NAME)" "$(TOOL_NAME)" > /dev/null

container-image: OS = linux
container-image: binary
	@docker buildx build --platform="linux/$(ARCH)" -t "$(TOOL_NAME)" .

clean:
	@rm -rf build/
	@if [ -n "$(shell docker image ls --quiet --filter "reference=$(TOOL_NAME):$(VERSION)" )" ]; then \
		docker image rm -f "$(TOOL_NAME):$(VERSION)"; \
	fi

.PHONY: lint test binary tarball container-image clean
