# altctl Makefile
# Build and development targets for the Alt platform CLI

BINARY_NAME := altctl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build clean test lint install help docs-man docs-markdown

## Build targets

all: build ## Default target: build the binary

build: ## Build the altctl binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) .

build-linux: ## Cross-compile for Linux amd64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 .

build-darwin: ## Cross-compile for macOS arm64
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 .

## Development targets

clean: ## Remove build artifacts
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*

test: ## Run tests
	go test -v ./...

test-coverage: ## Run tests with coverage
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	go fmt ./...
	goimports -w .

## Installation targets

install: build ## Install to GOPATH/bin
	go install $(LDFLAGS)

install-local: build ## Install to /usr/local/bin (requires sudo)
	sudo cp $(BINARY_NAME) /usr/local/bin/

## Completion scripts

completion-bash: build ## Generate bash completion script
	./$(BINARY_NAME) completion bash > $(BINARY_NAME).bash
	@echo "Source with: source $(BINARY_NAME).bash"

completion-zsh: build ## Generate zsh completion script
	./$(BINARY_NAME) completion zsh > _$(BINARY_NAME)
	@echo "Move to fpath: mv _$(BINARY_NAME) $${fpath[1]}/"

completion-fish: build ## Generate fish completion script
	./$(BINARY_NAME) completion fish > $(BINARY_NAME).fish
	@echo "Move to: mv $(BINARY_NAME).fish ~/.config/fish/completions/"

## Documentation targets

docs-man: build ## Generate man pages
	mkdir -p docs/man && ./$(BINARY_NAME) docs --format man --output docs/man

docs-markdown: build ## Generate markdown CLI reference
	mkdir -p docs/cli && ./$(BINARY_NAME) docs --format markdown --output docs/cli

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
