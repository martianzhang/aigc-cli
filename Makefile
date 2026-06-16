BINARY    ?= apimart-cli
GO        ?= go
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GOFLAGS   ?= -ldflags="-s -w -X github.com/martianzhang/apimart-cli/cmd.Version=$(VERSION)"
OUTPUT    ?= $(BINARY)
RELEASE_DIR   ?= dist
RELEASE_FLAGS ?= -ldflags="-s -w -X github.com/martianzhang/apimart-cli/cmd.Version=$(VERSION)" -trimpath

# Detect OS for output naming
ifeq ($(OS),Windows_NT)
	OUTPUT := $(BINARY).exe
endif

.PHONY: all build clean run lint vet test fmt cover release help

all: build

## Format all Go source code
fmt: 
	$(GO) fmt ./...

## Build the binary
build: fmt
	$(GO) build $(GOFLAGS) -o $(OUTPUT) .

## Build and run with args (usage: make run ARGS="image --help")
run: 
	$(GO) run . $(ARGS)

## Remove build artifacts
clean: 
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(RELEASE_DIR)

## Run static analysis
lint: 
	$(GO) vet ./...

vet: lint

## Run tests
test: fmt
	$(GO) test ./... -v -count=1

## Run tests with coverage report
cover: 
	$(GO) test ./... -cover -count=1
	@echo ""
	@echo "=== Detailed coverage ==="
	$(GO) test ./... -coverprofile=coverage.out -count=1
	$(GO) tool cover -func=coverage.out | tail -1
	@rm -f coverage.out


## Cross-compile for all targets into dist/
TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
release: 
	@mkdir -p $(RELEASE_DIR)
	@set -e; for target in $(TARGETS); do \
		os=$$(echo $$target | cut -d/ -f1); \
		arch=$$(echo $$target | cut -d/ -f2); \
		ext=; \
		[ "$$os" = "windows" ] && ext=.exe; \
		name=$(BINARY)-$$os-$$arch$$ext; \
		echo "  Building for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(RELEASE_FLAGS) -o $(RELEASE_DIR)/$$name .; \
	done
	@echo ""
	@echo "=== Release builds ready in $(RELEASE_DIR)/ ==="
	@ls -lh $(RELEASE_DIR)/

## Show this help
help: 
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
