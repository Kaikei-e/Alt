# Makefile for rask-log-forwarder

.PHONY: build test check lint docker-build docker-run clean release benchmark

# Build the application
build:
	cargo build

# Build release version
release:
	cargo build --release

# Run tests
test:
	cargo test

# Check compilation without building
check:
	cargo check

# Run linter
lint:
	cargo clippy --all-targets --all-features

# Format code
fmt:
	cargo fmt

# Run benchmarks
benchmark:
	cargo bench

# Build Docker image
docker-build:
	docker build -f Dockerfile.rask-log-forwarder -t rask-log-forwarder:latest .

# Run Docker container
docker-run:
	docker run --rm \
		-e TARGET_SERVICE=test \
		-e RASK_ENDPOINT=http://rask-aggregator:9600/v1/aggregate \
		-v /var/run/docker.sock:/var/run/docker.sock:ro \
		-v /var/lib/docker/containers:/var/lib/docker/containers:ro \
		rask-log-forwarder:latest

# Clean build artifacts
clean:
	cargo clean

# Development workflow
dev: fmt lint check test

# CI workflow
ci: lint check test

# Install development dependencies
install-deps:
	rustup component add clippy rustfmt

# Generate documentation
docs:
	cargo doc --open

# Run with debug logging
run-debug:
	RUST_LOG=debug cargo run -- --log-level debug --target-service test

# Run integration tests
test-integration:
	cargo test --test '*'

# Performance testing
perf-test: benchmark
	cargo test --release --test '*' -- --ignored

# Security audit
audit:
	cargo audit

help:
	@echo "Available targets:"
	@echo "  build       - Build the application"
	@echo "  release     - Build release version"
	@echo "  test        - Run tests"
	@echo "  check       - Check compilation"
	@echo "  lint        - Run clippy linter"
	@echo "  fmt         - Format code"
	@echo "  benchmark   - Run benchmarks"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run  - Run Docker container"
	@echo "  clean       - Clean build artifacts"
	@echo "  dev         - Development workflow (fmt + lint + check + test)"
	@echo "  ci          - CI workflow (lint + check + test)"
	@echo "  docs        - Generate documentation"
	@echo "  help        - Show this help"