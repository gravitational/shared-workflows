.PHONY: all
all: build

#
# build builds the bot binary
#
.PHONY: build
build:
	go build .

#
# test executes the bot's full test suite.
#
.PHONY: test
test:
	go test -v -count 1 -race ./...
