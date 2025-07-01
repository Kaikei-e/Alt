# GEMINI.md: rask-log-forwarder Service

This document provides best practices for the `rask-log-forwarder` service, adhering to Gemini standards as of July 2025. This is an ultra-high-performance sidecar container built with Rust 1.87+ (2024 edition) for log collection and forwarding.

## 1. Core Responsibilities

*   Tails `stdout`/`stderr` logs from its target service.
*   Performs zero-copy parsing of logs.
*   Forwards logs in batches to the Rask aggregation server.

## 2. Architecture

### 2.1. Deployment

*   **Sidecar Pattern**: One forwarder instance is deployed per service.
*   **Network Namespace Sharing**: Shares the network namespace of the target service for direct communication.
*   **Direct Log Access**: Mounts Docker's `json-file` logs for zero-copy reading.

### 2.2. Data Flow

**Docker json-file → Tail (Bollard) → Zero-copy Bytes → SIMD Parse → Lock-free Queue → Batch & Send**

## 3. High-Performance Implementation

*   **Zero-Copy Log Collection**: Uses `bollard` to tail Docker container logs and `Bytes` to avoid unnecessary data copying.
*   **SIMD-Accelerated Parsing**: Uses `simd-json` for high-throughput JSON parsing (>4 GB/s).
*   **Lock-Free Buffering**: Uses `multiqueue` for a lock-free MPMC queue that can handle millions of messages per second.
*   **Batch Transmission**: Uses `hyper`/`reqwest` with HTTP/1.1 keep-alive for low-latency batch transmission.
*   **Guaranteed Delivery**: Implements exponential backoff and disk fallback (using `sled`) for failed transmissions.

## 4. Development Guidelines

### 4.1. Test-Driven Development (TDD)

*   The Red-Green-Refactor cycle is mandatory.
*   Write failing tests before implementing any new functionality.

### 4.2. Rust 2024 Edition

*   Use the latest edition features and idioms.
*   Enable `clippy::pedantic` for strict linting.
*   Use `thiserror`/`anyhow` for error handling.

## 5. Configuration

*   Configuration is managed through a combination of CLI arguments and environment variables, unified with `clap`.
*   The target service is automatically detected from the hostname if not explicitly set.

## 6. Testing Strategy

*   **Unit Tests**: Test individual components in isolation.
*   **Integration Tests**: Test the interaction between the forwarder and a mock aggregation server.
*   **Performance Benchmarks**: Use `criterion` to benchmark critical code paths, such as SIMD JSON parsing.

## 7. Gemini Model Interaction

*   **Workspace Discovery**: Ensure that `Cargo.toml` is visible or `rust-analyzer.linkedProjects` is set correctly to avoid workspace discovery errors.
*   **TDD Micro-Diff Cadence**: Use a test-driven, micro-diff approach to generate and validate code.
*   **Borrow-Checker Repair Loop**: If the borrow checker produces errors, re-run `/fix-types` or prompt with a step-by-step thinking process to resolve the issue.