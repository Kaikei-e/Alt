.PHONY: test test-race coverage lint security format quality-check mocks clean help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=pre-processor
BINARY_PATH=bin/$(BINARY_NAME)

# Tools
GOPATH := $(shell go env GOPATH)
GOBIN := $(GOPATH)/bin
GOLANGCILINT := $(GOBIN)/golangci-lint
GOSEC := $(GOBIN)/gosec
GOIMPORTS := $(GOBIN)/goimports

# Default target
all: quality-check build

# Help target
help:
	@echo "Pre-processor Service Build Commands:"
	@echo "  make test           - Run fast unit tests"
	@echo "  make test-race      - Run tests with race detection"
	@echo "  make coverage       - Generate test coverage report"
	@echo "  make lint           - Run linter"
	@echo "  make security       - Run security scanner"
	@echo "  make format         - Format code"
	@echo "  make mocks          - Generate mocks"
	@echo "  make quality-check  - Run all quality checks"
	@echo "  make build          - Build the application"
	@echo "  make clean          - Clean build artifacts"

# Run fast unit tests only (default)
test:
	@echo "🧪 Running fast unit tests..."
	$(GOTEST) -v ./...

# Test with race detection
test-race:
	@echo "🏃 Running tests with race detection..."
	$(GOTEST) -race -v ./...

# Integration tests (requires real database)
test-integration:
	@echo "🔗 Running integration tests (requires DB)..."
	@echo "⚠️  Make sure database is running locally"
	$(GOTEST) -v ./test/integration/...

# Generate coverage report
coverage:
	@echo "📊 Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@coverage=$$($(GOCMD) tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Coverage: $$coverage%"; \
	if [ $$(echo "$$coverage < 80" | bc -l) -eq 1 ]; then \
		echo "❌ Coverage $$coverage% is below 80% threshold"; \
		exit 1; \
	else \
		echo "✅ Coverage $$coverage% meets 80% threshold"; \
	fi

# Lint code
lint:
	@echo "🔍 Running linter..."
	@$(GOLANGCILINT) run ./...

# Security scan
security:
	@echo "🔒 Running security scan..."
	@$(GOSEC) ./...

# Format code
format:
	@echo "🎨 Formatting code..."
	$(GOCMD) fmt ./...
	@$(GOIMPORTS) -w .

# Generate mocks
mocks:
	@echo "🤖 Generating mocks..."
	@PATH=$(GOBIN):$(PATH) $(GOCMD) generate ./...

# Build the application
build:
	@echo "🔨 Building application..."
	$(GOBUILD) -o $(BINARY_PATH) .

# Run all quality checks (MANDATORY before commit)
quality-check: format lint security test-race coverage
	@echo "🎉 All quality checks passed! ✅"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	$(GOCLEAN)
	rm -f $(BINARY_PATH)
	rm -f coverage.out
	rm -f coverage.html

# Development helpers
dev-setup:
	@echo "🚀 Setting up development environment..."
	$(GOMOD) tidy
	$(GOMOD) download
	@echo "Installing development tools..."
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOCMD) install github.com/securego/gosec/v2/cmd/gosec@latest
	$(GOCMD) install go.uber.org/mock/mockgen@latest
	$(GOCMD) install golang.org/x/tools/cmd/goimports@latest
	@echo "Development environment ready! ✅"

# Docker targets
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t pre-processor:latest .

docker-run:
	@echo "🐳 Running Docker container..."
	docker run --rm -p 8080:8080 pre-processor:latest

# Dependency management
deps:
	@echo "📦 Updating dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# Check for vulnerabilities
vuln-check:
	@echo "🛡️ Checking for vulnerabilities..."
	$(GOCMD) list -json -deps ./... | nancy sleuth

# Pre-commit hook (run this before every commit)
pre-commit: quality-check
	@echo "✅ Pre-commit checks passed! Ready to commit."

# CI/CD targets
ci: quality-check build
	@echo "✅ CI pipeline completed successfully!"