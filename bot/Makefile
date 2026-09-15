.PHONY: all
all: build

#
# build builds the bot binary
#
.PHONY: build
build:
	go build .

#
# lint runs the linter over the bot's source.
#
.PHONY: lint
lint:
	golangci-lint run ./...

#
# test executes the bot's full test suite.
#
.PHONY: test
test:
	go test -v -count 1 -race ./...
