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

.PHONY: all build clean run install lint vet test test-race test-integration fmt cover cover-check bench bench-baseline bench-check size release help ideas docs-check docs-gen docs-gen-check lint-readall tidy-check vuln pre-commit

all: build

fmt: ## Format all Go source code
	$(GO) fmt ./...

build: fmt ## Build the binary
	$(GO) build $(LDFLAGS) -o $(OUTPUT) .

install: fmt ## Install into $GOPATH/bin
	$(GO) install $(LDFLAGS) ./...

ideas: ## Ensure ideas.json exists (script skips if already up-to-date)
	python scripts/convert_ideas.py || python3 scripts/convert_ideas.py

run: ## Build and run with args (usage: make run ARGS="image --help")
	$(GO) run . $(ARGS)

clean: ## Remove build artifacts
	rm -f $(sort $(OUTPUT) $(BINARY) $(BINARY).exe)
	rm -rf $(RELEASE_DIR)

lint: lint-readall ## Run static analysis (go vet + golangci-lint if available)
	$(GO) vet ./...
	$(if $(GOLANGCI_LINT),golangci-lint run ./...,@echo "  golangci-lint not installed, skipping")

lint-readall: ## Fail when io.ReadAll is neither capped nor justified
	python3 scripts/check_readall.py

vet: lint

test: fmt ## Run hermetic tests (integration tests skip via -short)
	$(GO) test ./... -short -count=1

test-race: fmt ## Run tests with the race detector
	$(GO) test -race ./... -count=1 -timeout=25m

test-integration: fmt ## Run integration tests (no -short; build tag: integration)
	$(GO) test -tags=integration ./... -count=1 -timeout=30m

bench: ## Run benchmarks (optional: BENCH=ClipString)
	$(GO) test ./... -run '^$$' -bench '$(if $(BENCH),$(BENCH),.)' -benchmem -count=1

BENCH_PKGS ?= ./internal/cli/chat ./internal/agent ./internal/session
BENCHSTAT  ?= $(GO) run golang.org/x/perf/cmd/benchstat@latest

bench-baseline: ## Record a benchmark baseline (benchmarks/baseline.txt)
	@mkdir -p benchmarks
	$(GO) test $(BENCH_PKGS) -run '^$$' -bench . -benchmem -count=5 > benchmarks/baseline.txt

bench-check: ## Compare current benchmarks with benchmarks/baseline.txt
	@test -f benchmarks/baseline.txt || { echo "run 'make bench-baseline' first"; exit 1; }
	$(GO) test $(BENCH_PKGS) -run '^$$' -bench . -benchmem -count=5 > .bench-new.txt
	$(BENCHSTAT) benchmarks/baseline.txt .bench-new.txt
	@rm -f .bench-new.txt

size: build ## Report the release binary size
	@ls -lh $(OUTPUT)
	@echo "Build with an optional slim tag set in a future release if local ONNX/wasm is not needed."

cover: fmt ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out -count=1
	@echo ""
	@echo "=== Detailed coverage ==="
	$(GO) tool cover -func=coverage.out | tail -1
	@rm -f coverage.out

COVER_MIN ?= 50

cover-check: fmt ## Fail when hermetic (-short) coverage drops below COVER_MIN (default 50%)
	$(GO) test ./... -short -coverprofile=coverage.out -count=1
	@total=$$($(GO) tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | tr -d '%'); \
	echo "total coverage: $$total% (floor: $(COVER_MIN)%)"; \
	awk -v total="$$total" -v min="$(COVER_MIN)" 'BEGIN { if (total + 0 < min + 0) { print "coverage is below the floor"; exit 1 } }'
	@rm -f coverage.out

docs-gen: ## Regenerate the MCP tool catalog
	$(GO) run ./scripts/gen-mcp-tools > docs/reference/mcp-tools.md

docs-gen-check: ## Fail when the generated MCP tool catalog is stale
	@tmp=$$(mktemp); trap 'rm -f "$$tmp"' EXIT; \
	$(GO) run ./scripts/gen-mcp-tools > "$$tmp"; \
	diff -u docs/reference/mcp-tools.md "$$tmp"

docs-check: docs-gen-check ## Check generated docs, links and zh/en parity
	python3 scripts/check_docs.py

tidy-check: ## Fail when go.mod/go.sum are not tidy
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum

vuln: ## Run govulncheck
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest ./...

pre-commit: fmt lint test docs-check tidy-check ## Local gate before committing

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
