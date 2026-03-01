APP_NAME := i9c
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
GOCACHE_DIR := $(CURDIR)/.i9c/cache/go-build

.PHONY: prep build run run-debug vet test regression smoke release-check release-snapshot clean

prep:
	mkdir -p $(GOCACHE_DIR)

build: prep
	GOCACHE=$(GOCACHE_DIR) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/i9c/

run: build
	$(BUILD_DIR)/$(APP_NAME)

run-debug: build
	I9C_NO_ALT_SCREEN=1 $(BUILD_DIR)/$(APP_NAME)

vet:
	mkdir -p $(GOCACHE_DIR)
	go vet ./...
	GOCACHE=$(GOCACHE_DIR) go build ./...

test: prep
	GOCACHE=$(GOCACHE_DIR) go test ./...

regression: prep
	GOCACHE=$(GOCACHE_DIR) go test ./...
	GOCACHE=$(GOCACHE_DIR) go test -run Test -count=1 ./internal/aws ./internal/mcp ./internal/tui/views ./internal/terraform ./internal/config ./internal/codexbridge ./internal/app

smoke: prep
	@echo "Running deterministic smoke checks"
	GOCACHE=$(GOCACHE_DIR) go test -run TestTryStartDriftRunDebounce ./internal/app
	GOCACHE=$(GOCACHE_DIR) go test -run TestDiscoverFallsBackToSecondary ./internal/mcp

release-check:
	@command -v goreleaser >/dev/null 2>&1 || (echo "goreleaser not found. Install from https://goreleaser.com/install/"; exit 1)
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) goreleaser check

release-snapshot:
	@command -v goreleaser >/dev/null 2>&1 || (echo "goreleaser not found. Install from https://goreleaser.com/install/"; exit 1)
	mkdir -p $(GOCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) goreleaser release --snapshot --clean

clean:
	rm -rf $(BUILD_DIR)
