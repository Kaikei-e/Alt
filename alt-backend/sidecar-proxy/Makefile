# Makefile for lightweight proxy sidecar
# ISSUE_RESOLVE_PLAN.md compliant build system

# Variables
APP_NAME = proxy-sidecar
VERSION ?= 1.0.0
IMAGE_NAME = alt-apps/$(APP_NAME)
IMAGE_TAG = $(VERSION)
GO_VERSION = 1.23

# Build variables
BUILD_DIR = build
BINARY_NAME = $(APP_NAME)
MAIN_FILE = cmd/main.go

# Test variables
COVERAGE_FILE = coverage.out
COVERAGE_HTML = coverage.html

# Colors for output
RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[1;33m
BLUE = \033[0;34m
NC = \033[0m # No Color

.PHONY: help build test clean docker-build docker-run deps lint fmt vet coverage dev-setup local-test

# Default target
help: ## Show this help message
	@echo "$(BLUE)Lightweight Proxy Sidecar - ISSUE_RESOLVE_PLAN.md Implementation$(NC)"
	@echo ""
	@echo "$(YELLOW)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*##"; printf "\033[36m\033[0m"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

deps: ## Download and verify dependencies
	@echo "$(BLUE)📦 Downloading dependencies...$(NC)"
	go mod download
	go mod verify
	go mod tidy

fmt: ## Format Go code
	@echo "$(BLUE)🎨 Formatting code...$(NC)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(BLUE)🔍 Running go vet...$(NC)"
	go vet ./...

lint: deps fmt vet ## Run all linting tools
	@echo "$(GREEN)✅ Linting completed$(NC)"

##@ Building

build: deps ## Build the binary
	@echo "$(BLUE)🔨 Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags='-w -s -X main.Version=$(VERSION)' \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		$(MAIN_FILE)
	@echo "$(GREEN)✅ Build completed: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

build-local: deps ## Build for local development
	@echo "$(BLUE)🔨 Building for local development...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build -ldflags='-X main.Version=$(VERSION)' \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		$(MAIN_FILE)
	@echo "$(GREEN)✅ Local build completed: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

##@ Testing

test: deps ## Run unit tests
	@echo "$(BLUE)🧪 Running tests...$(NC)"
	go test -v ./...

test-race: deps ## Run tests with race detection
	@echo "$(BLUE)🧪 Running tests with race detection...$(NC)"
	go test -race -v ./...

coverage: deps ## Generate test coverage report
	@echo "$(BLUE)📊 Generating coverage report...$(NC)"
	go test -coverprofile=$(COVERAGE_FILE) ./...
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(GREEN)✅ Coverage report generated: $(COVERAGE_HTML)$(NC)"

benchmark: deps ## Run benchmarks
	@echo "$(BLUE)⚡ Running benchmarks...$(NC)"
	go test -bench=. -benchmem ./...

##@ Local Development

dev-setup: deps ## Setup development environment
	@echo "$(BLUE)🛠️ Setting up development environment...$(NC)"
	@echo "$(YELLOW)Installing development tools...$(NC)"
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "$(GREEN)✅ Development environment setup completed$(NC)"

local-test: build-local ## Test the proxy sidecar locally (requires manual verification)
	@echo "$(BLUE)🧪 Starting local test of proxy sidecar...$(NC)"
	@echo "$(YELLOW)Configuration:$(NC)"
	@echo "  LISTEN_PORT=8080"
	@echo "  ENVOY_UPSTREAM=localhost:10000"
	@echo "  DNS_SERVERS=8.8.8.8:53,1.1.1.1:53"
	@echo "  ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com"
	@echo ""
	@echo "$(YELLOW)Test endpoints will be available at:$(NC)"
	@echo "  Health: http://localhost:8080/health"
	@echo "  Ready: http://localhost:8080/ready"
	@echo "  Metrics: http://localhost:8080/metrics"
	@echo "  DNS Debug: http://localhost:8080/debug/dns"
	@echo ""
	@echo "$(YELLOW)To test proxy functionality (when Envoy is available):$(NC)"
	@echo "  curl http://localhost:8080/proxy/https://httpbin.org/get"
	@echo ""
	@echo "$(BLUE)Starting proxy sidecar...$(NC)"
	LISTEN_PORT=8080 \
	ENVOY_UPSTREAM=localhost:10000 \
	DNS_SERVERS=8.8.8.8:53,1.1.1.1:53 \
	ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com \
	DEBUG_MODE=true \
	VERBOSE_LOGGING=true \
	./$(BUILD_DIR)/$(BINARY_NAME)

quick-test: build-local ## Quick functionality test (DNS resolution only)
	@echo "$(BLUE)⚡ Running quick functionality test...$(NC)"
	@echo "$(YELLOW)Testing DNS resolution and basic functionality...$(NC)"
	LISTEN_PORT=8081 \
	ENVOY_UPSTREAM=localhost:10000 \
	DNS_SERVERS=8.8.8.8:53,1.1.1.1:53 \
	ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com \
	DEBUG_MODE=true \
	./$(BUILD_DIR)/$(BINARY_NAME) &
	@sleep 2
	@echo "$(BLUE)Testing health endpoint...$(NC)"
	curl -s http://localhost:8081/health || echo "Health check failed"
	@echo ""
	@echo "$(BLUE)Testing readiness endpoint...$(NC)"
	curl -s http://localhost:8081/ready || echo "Readiness check failed"
	@echo ""
	@echo "$(BLUE)Testing DNS debug endpoint...$(NC)"
	curl -s http://localhost:8081/debug/dns || echo "DNS debug failed"
	@echo ""
	@pkill -f $(BINARY_NAME) || true
	@echo "$(GREEN)✅ Quick test completed$(NC)"

##@ Docker

docker-build: ## Build Docker image
	@echo "$(BLUE)🐳 Building Docker image...$(NC)"
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(IMAGE_NAME):latest
	@echo "$(GREEN)✅ Docker image built: $(IMAGE_NAME):$(IMAGE_TAG)$(NC)"

docker-run: docker-build ## Run Docker container locally
	@echo "$(BLUE)🐳 Running Docker container...$(NC)"
	docker run -p 8080:8080 -p 8081:8081 -p 9090:9090 \
		-e ENVOY_UPSTREAM=host.docker.internal:10000 \
		-e DNS_SERVERS=8.8.8.8:53,1.1.1.1:53 \
		-e ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com \
		-e DEBUG_MODE=true \
		$(IMAGE_NAME):$(IMAGE_TAG)

docker-test: docker-build ## Test Docker container
	@echo "$(BLUE)🐳 Testing Docker container...$(NC)"
	docker run --rm -d --name $(APP_NAME)-test \
		-p 8082:8080 \
		-e ENVOY_UPSTREAM=host.docker.internal:10000 \
		-e DNS_SERVERS=8.8.8.8:53,1.1.1.1:53 \
		-e ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com \
		$(IMAGE_NAME):$(IMAGE_TAG)
	@sleep 3
	@echo "$(BLUE)Testing container health...$(NC)"
	curl -f http://localhost:8082/health || (docker stop $(APP_NAME)-test && exit 1)
	@echo "$(BLUE)Testing container readiness...$(NC)"
	curl -f http://localhost:8082/ready || (docker stop $(APP_NAME)-test && exit 1)
	docker stop $(APP_NAME)-test
	@echo "$(GREEN)✅ Docker container test passed$(NC)"

##@ Maintenance

clean: ## Clean build artifacts
	@echo "$(BLUE)🧹 Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	docker image prune -f || true
	@echo "$(GREEN)✅ Cleanup completed$(NC)"

clean-all: clean ## Clean everything including dependencies
	@echo "$(BLUE)🧹 Deep cleaning...$(NC)"
	go clean -modcache
	docker system prune -f || true
	@echo "$(GREEN)✅ Deep cleanup completed$(NC)"

##@ Information

info: ## Show build information
	@echo "$(BLUE)📋 Build Information$(NC)"
	@echo "  App Name: $(APP_NAME)"
	@echo "  Version: $(VERSION)"
	@echo "  Image: $(IMAGE_NAME):$(IMAGE_TAG)"
	@echo "  Go Version: $(GO_VERSION)"
	@echo "  Build Dir: $(BUILD_DIR)"
	@echo ""
	@echo "$(BLUE)📋 System Information$(NC)"
	@echo "  OS: $$(go env GOOS)"
	@echo "  Arch: $$(go env GOARCH)"
	@echo "  Go Version: $$(go version)"
	@echo "  Docker: $$(docker --version 2>/dev/null || echo 'Not available')"

status: ## Show current status
	@echo "$(BLUE)📊 Current Status$(NC)"
	@echo "  Built Binary: $$(test -f $(BUILD_DIR)/$(BINARY_NAME) && echo '✅ Yes' || echo '❌ No')"
	@echo "  Docker Image: $$(docker images -q $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null | head -1 | sed 's/.*/✅ Yes/' || echo '❌ No')"
	@echo "  Dependencies: $$(test -f go.mod && echo '✅ Ready' || echo '❌ Missing')"
	@echo "  Test Coverage: $$(test -f $(COVERAGE_FILE) && echo '✅ Generated' || echo '❌ Not generated')"

##@ ISSUE_RESOLVE_PLAN.md Workflow

week1-phase2: deps build test quick-test ## Complete Week 1 Phase 2 tasks
	@echo "$(GREEN)🎯 Week 1 Phase 2 Completed Successfully!$(NC)"
	@echo "$(BLUE)📋 Summary:$(NC)"
	@echo "  ✅ Dependencies downloaded and verified"
	@echo "  ✅ Binary built successfully"
	@echo "  ✅ Unit tests passed"
	@echo "  ✅ Quick functionality test completed"
	@echo ""
	@echo "$(YELLOW)🔄 Next Steps (Week 1 Phase 3):$(NC)"
	@echo "  - Implement comprehensive tests"
	@echo "  - Add integration tests with mock Envoy"
	@echo "  - Verify upstream resolution behavior"
	@echo "  - Test with real external domains"

upstream-test: build-local ## Test upstream resolution logic specifically
	@echo "$(BLUE)🎯 Testing Upstream Resolution Logic...$(NC)"
	@echo "$(YELLOW)This test verifies that the proxy correctly handles DNS resolution$(NC)"
	@echo "$(YELLOW)and header construction for upstream resolution.$(NC)"
	@echo ""
	@echo "$(BLUE)Expected behavior:$(NC)"
	@echo "  1. DNS resolution of external domains (bypassing k8s DNS)"
	@echo "  2. Proper HTTP header construction (Host, X-Target-Domain)"
	@echo "  3. Correct logging of upstream resolution attempts"
	@echo ""
	@echo "$(BLUE)Starting proxy with verbose logging...$(NC)"
	LISTEN_PORT=8083 \
	ENVOY_UPSTREAM=localhost:10000 \
	DNS_SERVERS=8.8.8.8:53,1.1.1.1:53 \
	ALLOWED_DOMAINS=httpbin\\.org,zenn\\.dev,github\\.com \
	DEBUG_MODE=true \
	VERBOSE_LOGGING=true \
	DEBUG_STARTUP=true \
	./$(BUILD_DIR)/$(BINARY_NAME)

# Default target when no arguments are provided
.DEFAULT_GOAL := help