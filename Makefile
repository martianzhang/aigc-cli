BINARY    ?= apimart-cli
GO        ?= go
GOFLAGS   ?= -ldflags="-s -w"
OUTPUT    ?= $(BINARY)
RELEASE_DIR   ?= dist
RELEASE_FLAGS ?= -ldflags="-s -w" -trimpath

# Detect OS for output naming
ifeq ($(OS),Windows_NT)
	OUTPUT := $(BINARY).exe
endif

.PHONY: all build clean run lint vet test fmt cover release help

all: build

fmt: ## Format all Go source code
	$(GO) fmt ./...

build: fmt ## Build the binary
	$(GO) build $(GOFLAGS) -o $(OUTPUT) .

run: ## Build and run with args (usage: make run ARGS="image --help")
	$(GO) run . $(ARGS)

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe
	rm -rf $(RELEASE_DIR)

lint: ## Run static analysis
	$(GO) vet ./...

vet: lint

test: ## Run tests
	$(GO) test ./... -v -count=1

cover: ## Run tests with coverage report
	$(GO) test ./... -cover -count=1
	@echo ""
	@echo "=== Detailed coverage ==="
	$(GO) test ./... -coverprofile=coverage.out -count=1
	$(GO) tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

release: ## Cross-compile for all targets into dist/
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

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
