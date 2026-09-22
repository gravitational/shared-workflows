.PHONY: all
all: test

#
# lint runs the linter over the shared libraries' source.
#
.PHONY: lint
lint:
	golangci-lint run ./...

#
# test executes the shared libraries' full test suite.
#
.PHONY: test
test:
	go test -v -count 1 -race ./...
