# Makefile for deploy-cli

# Build variables
BINARY_NAME=deploy-cli
CMD_DIR=./cmd
BUILD_DIR=./build
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Default target
.PHONY: all
all: clean deps fmt test build

# Build the binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for current directory
.PHONY: build-local
build-local:
	@echo "Building $(BINARY_NAME) in current directory..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: ./$(BINARY_NAME)"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies updated"

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...
	@echo "Formatting complete"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Lint code
.PHONY: lint
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# Install the binary to GOPATH/bin
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to GOPATH/bin..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/
	@echo "Installation complete"

# Cross-compile for multiple platforms
.PHONY: build-cross
build-cross: clean
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	
	# Linux AMD64
	@echo "Building for Linux AMD64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	
	# Linux ARM64
	@echo "Building for Linux ARM64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)
	
	# macOS AMD64
	@echo "Building for macOS AMD64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	
	# macOS ARM64
	@echo "Building for macOS ARM64..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	
	# Windows AMD64
	@echo "Building for Windows AMD64..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	
	@echo "Cross-compilation complete"
	@ls -la $(BUILD_DIR)/

# Development build with race detection
.PHONY: build-dev
build-dev:
	@echo "Building development version with race detection..."
	$(GOBUILD) -race $(LDFLAGS) -o $(BINARY_NAME)-dev $(CMD_DIR)
	@echo "Development build complete: ./$(BINARY_NAME)-dev"

# Quick validation (fmt + test + build)
.PHONY: validate
validate: fmt test build-local
	@echo "Validation complete"

# Help target
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         - Build the binary to $(BUILD_DIR)/$(BINARY_NAME)"
	@echo "  build-local   - Build the binary to ./$(BINARY_NAME)"
	@echo "  build-cross   - Cross-compile for multiple platforms"
	@echo "  build-dev     - Build development version with race detection"
	@echo "  clean         - Clean build artifacts"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  fmt           - Format code"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run linter (requires golangci-lint)"
	@echo "  install       - Install binary to GOPATH/bin"
	@echo "  validate      - Quick validation (fmt + test + build)"
	@echo "  all           - Run clean + deps + fmt + test + build"
	@echo "  help          - Show this help message"

# Default target if no target is specified
.DEFAULT_GOAL := help