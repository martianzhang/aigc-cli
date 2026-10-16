BINARY    ?= aigc-cli
GO        ?= go
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   ?= -ldflags="-s -w -X github.com/martianzhang/aigc-cli/cmd.Version=$(VERSION)"
RELEASE_DIR   ?= dist
RELEASE_FLAGS ?= -ldflags="-s -w -X github.com/martianzhang/aigc-cli/cmd.Version=$(VERSION)" -trimpath

# Output name follows the target GOOS (also honors GOOS=windows make build).
GOEXE  := $(shell $(GO) env GOEXE)
OUTPUT ?= $(BINARY)$(GOEXE)

# Resolve golangci-lint once (empty when not installed).
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)

.PHONY: all build clean run install lint vet test fmt cover release help ideas

all: build

fmt: ## Format all Go source code
	$(GO) fmt ./...

build: fmt ## Build the binary
	$(GO) build $(LDFLAGS) -o $(OUTPUT) .

install: fmt ## Install into $GOPATH/bin
	$(GO) install ./...

ideas: ## Ensure ideas.json exists (script skips if already up-to-date)
	python scripts/convert_ideas.py || python3 scripts/convert_ideas.py

run: ## Build and run with args (usage: make run ARGS="image --help")
	$(GO) run . $(ARGS)

clean: ## Remove build artifacts
	rm -f $(sort $(OUTPUT) $(BINARY) $(BINARY).exe)
	rm -rf $(RELEASE_DIR)

lint: ## Run static analysis (go vet + golangci-lint if available)
	$(GO) vet ./...
	$(if $(GOLANGCI_LINT),golangci-lint run ./...,@echo "  golangci-lint not installed, skipping")

vet: lint

test: fmt ## Run tests
	$(GO) test ./... -v -count=1

cover: fmt ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out -count=1
	@echo ""
	@echo "=== Detailed coverage ==="
	$(GO) tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

## Cross-compile the Go binary for all targets into dist/ (native helper/libs are packaged by CI)
TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
release: ## Cross-compile Go binaries into dist/ (native libs packaged by CI)
	@echo "  Note: builds the Go binary only; native helper/libs are packaged by CI (.github/workflows/ci.yml)."
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
