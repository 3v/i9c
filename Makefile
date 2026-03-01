APP_NAME := i9c
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
GOCACHE_DIR := $(PWD)/.i9c/cache/go-build

.PHONY: build run run-debug vet test regression smoke clean

build:
	GOCACHE=$(GOCACHE_DIR) go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/i9c/

run: build
	$(BUILD_DIR)/$(APP_NAME)

run-debug: build
	I9C_NO_ALT_SCREEN=1 $(BUILD_DIR)/$(APP_NAME)

vet:
	go vet ./...
	GOCACHE=$(GOCACHE_DIR) go build ./...

test:
	GOCACHE=$(GOCACHE_DIR) go test ./...

regression:
	GOCACHE=$(GOCACHE_DIR) go test ./...
	GOCACHE=$(GOCACHE_DIR) go test -run Test -count=1 ./internal/aws ./internal/mcp ./internal/tui/views ./internal/terraform ./internal/config ./internal/codexbridge ./internal/app

smoke:
	@echo "Running deterministic smoke checks"
	GOCACHE=$(GOCACHE_DIR) go test -run TestTryStartDriftRunDebounce ./internal/app
	GOCACHE=$(GOCACHE_DIR) go test -run TestDiscoverFallsBackToSecondary ./internal/mcp

clean:
	rm -rf $(BUILD_DIR)
