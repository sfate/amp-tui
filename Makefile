.PHONY: lint audit test build dev-build ci

BINARY_NAME ?= amp-tui
BUILD_OUTPUT ?= bin/$(BINARY_NAME)
DEV_BUILD_OUTPUT ?= $(HOME)/.local/bin/$(BINARY_NAME)
TEST_DIR ?= ./...
TEST_CASE ?= ^.+$
UNAME_S := $(shell uname -s)

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run --allow-parallel-runners

audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test:
	go test -mod=readonly -count=1 -p 1 -failfast -race -run $(TEST_CASE) $(TEST_DIR)

build:
	mkdir -p $(dir $(BUILD_OUTPUT))
	go build -mod=readonly -o $(BUILD_OUTPUT) ./cmd/amp-tui

dev-build: build
	mkdir -p "$(dir $(DEV_BUILD_OUTPUT))"
	ln -sf "$(PWD)/$(BUILD_OUTPUT)" "$(DEV_BUILD_OUTPUT)"

ci: lint audit test
