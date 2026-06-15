BINARY   ?= apimart-cli
GO       ?= go
GOFLAGS  ?= -ldflags="-s -w"
OUTPUT   ?= $(BINARY)

# Detect OS for output naming
ifeq ($(OS),Windows_NT)
	OUTPUT := $(BINARY).exe
endif

.PHONY: all build clean run lint vet test help

all: build

build: ## Build the binary
	$(GO) build $(GOFLAGS) -o $(OUTPUT) .

run: ## Build and run with args (usage: make run ARGS="generate --help")
	$(GO) run . $(ARGS)

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe

lint: ## Run static analysis
	$(GO) vet ./...

vet: lint

test: ## Run tests (none yet)
	$(GO) test ./... -v -count=1

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
