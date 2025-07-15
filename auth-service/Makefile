# Makefile for Auth Service

.PHONY: help build test test-coverage run clean lint fmt deps-install deps-update deps-tidy docker-build docker-run

# Variables
BINARY_NAME=auth-service
BUILD_DIR=bin
MAIN_PATH=./app/cmd/server

# Go variables
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Version
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.commit=$(COMMIT)"

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the application
	@echo "Building $(BINARY_NAME) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v -tags=integration ./tests/integration/...

test-e2e: ## Run end-to-end tests
	@echo "Running end-to-end tests..."
	@go test -v -tags=e2e ./tests/e2e/...

run: ## Run the application
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH)

run-dev: ## Run the application in development mode
	@echo "Running $(BINARY_NAME) in development mode..."
	@LOG_LEVEL=debug go run $(MAIN_PATH)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@go clean

lint: ## Run linter
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run; \
	fi

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...
	@go mod tidy

deps-install: ## Install dependencies
	@echo "Installing dependencies..."
	@go mod download

deps-update: ## Update dependencies
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy

deps-tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	@go mod tidy

generate: ## Generate code (mocks, etc.)
	@echo "Generating code..."
	@go generate ./...

# Database operations
db-migrate-up: ## Run database migrations up
	@echo "Running database migrations up..."
	@./migration/01_init_auth_db.sh

db-migrate-down: ## Run database migrations down
	@echo "Running database migrations down..."
	@./migration/04_rollback.sh

db-migrate-verify: ## Verify database migrations
	@echo "Verifying database migrations..."
	@./migration/03_verify_migration.sh

# Docker operations
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	@docker build -t $(BINARY_NAME):$(VERSION) .

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	@docker run -p 9500:9500 --env-file .env $(BINARY_NAME):$(VERSION)

# Development helpers
dev-setup: ## Set up development environment
	@echo "Setting up development environment..."
	@cp .env.example .env
	@echo "Please edit .env with your configuration"
	@$(MAKE) deps-install

hot-reload: ## Run with hot reload (requires air)
	@echo "Running with hot reload..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not installed. Installing..."; \
		go install github.com/air-verse/air@latest; \
		air; \
	fi

# Security
security-scan: ## Run security scan
	@echo "Running security scan..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed. Installing..."; \
		go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; \
		gosec ./...; \
	fi

# Documentation
docs: ## Generate documentation
	@echo "Generating documentation..."
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g $(MAIN_PATH)/main.go; \
	else \
		echo "swag not installed. Installing..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		swag init -g $(MAIN_PATH)/main.go; \
	fi

# CI/CD helpers
ci-test: ## Run tests in CI environment
	@echo "Running CI tests..."
	@go test -v -race -coverprofile=coverage.out ./...

ci-lint: ## Run linter in CI environment
	@echo "Running CI linter..."
	@golangci-lint run --timeout=5m

ci-security: ## Run security checks in CI environment
	@echo "Running CI security checks..."
	@gosec -quiet ./...

# Multi-platform build
build-all: ## Build for all platforms
	@echo "Building for all platforms..."
	@$(MAKE) build GOOS=linux GOARCH=amd64
	@$(MAKE) build GOOS=darwin GOARCH=amd64
	@$(MAKE) build GOOS=windows GOARCH=amd64

# Help with environment variables
env-help: ## Show environment variables help
	@echo "Required environment variables:"
	@echo "  DATABASE_URL          - PostgreSQL connection string"
	@echo "  DB_PASSWORD           - Database password"
	@echo "  KRATOS_PUBLIC_URL     - Ory Kratos public URL"
	@echo "  KRATOS_ADMIN_URL      - Ory Kratos admin URL"
	@echo ""
	@echo "Optional environment variables:"
	@echo "  PORT                  - Server port (default: 9500)"
	@echo "  HOST                  - Server host (default: 0.0.0.0)"
	@echo "  LOG_LEVEL             - Log level (default: info)"
	@echo "  DB_HOST               - Database host (default: auth-postgres)"
	@echo "  DB_PORT               - Database port (default: 5432)"
	@echo "  DB_NAME               - Database name (default: auth_db)"
	@echo "  DB_USER               - Database user (default: auth_user)"
	@echo "  CSRF_TOKEN_LENGTH     - CSRF token length (default: 32)"
	@echo "  SESSION_TIMEOUT       - Session timeout (default: 24h)"
	@echo "  ENABLE_AUDIT_LOG      - Enable audit logging (default: true)"
	@echo "  ENABLE_METRICS        - Enable metrics (default: true)"
	@echo "  GO_ENV                - Environment (development/production)"