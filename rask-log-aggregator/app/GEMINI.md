# GEMINI.md: rask-log-aggregator Service

This document outlines the best practices for the `rask-log-aggregator` service, adhering to Gemini standards as of July 2025. This service is a high-performance log aggregation and processing system built with Rust 1.87+ and the Axum framework.

## 1. Core Responsibilities

*   Real-time ingestion of logs from multiple microservices.
*   High-throughput log parsing, enrichment, and analysis.
*   Structured log storage and indexing in a time-series database (ClickHouse) for efficient querying and analysis.
*   Real-time alerting and anomaly detection.

## 2. Architecture

### 2.1. Five-Layer Clean Architecture

**REST/gRPC Handler → Usecase → Port → Gateway (ACL) → Driver**

*   **Driver**: Integrates with external systems like Kafka, ClickHouse, Redis, and S3.

### 2.2. Data Flow

**Log Source (e.g., `rask-log-forwarder`) → HTTP Ingestion → Log Parsing & Enrichment → ClickHouse Batch Insertion**

### 2.3. Performance-Critical Design

*   **Zero-Copy Processing**: Use `bytes::Bytes` and `nom` for efficient, zero-copy log parsing.
*   **Lock-Free Data Structures**: Use `crossbeam` channels and `dashmap` for high-throughput, concurrent data handling.
*   **Vectorized Processing**: Use `rayon` for parallel processing of log batches.

## 3. Development Guidelines

### 3.1. Test-Driven Development (TDD)

*   The Red-Green-Refactor cycle is mandatory for all code changes.
*   Write failing tests before implementing any new functionality.

### 3.2. Rust 2024 Edition

*   Use `async fn` and return-position `impl Trait` directly in traits.
*   Eliminate `static mut` in favor of safer alternatives like `OnceCell` or `Mutex`.
*   Enforce edition hygiene with `#![deny(warnings, rust_2024_idioms)]`.

## 4. High-Performance Patterns

### 4.1. Log Ingestion

*   Use Axum for batch ingestion over HTTP and WebSockets for real-time streaming.
*   Parse logs without copying data by using `Bytes` and efficient parsing libraries.
*   **Batch Insertion**: Implement efficient batch insertion into ClickHouse to minimize network overhead and maximize throughput.

### 4.2. Log Processing Pipeline

*   Use `futures::stream` to create a processing pipeline that can parse, enrich, and index logs concurrently.
*   Buffer unordered operations to maximize parallelism.

### 4.3. Real-time Analytics

*   Use `dashmap` for lock-free, concurrent metric collection.
*   Track metrics like service-specific log counts and error rates in real-time.

## 5. Testing Strategy

*   **Unit Tests**: Test individual components and logic in isolation.
*   **Integration Tests**: Test the interaction between different components of the service.
*   **Performance Tests**: Use `criterion` to benchmark critical code paths and ensure performance targets are met.
*   **Load Tests**: Simulate high-throughput scenarios to verify the system's stability and performance under load.

## 6. Monitoring and Observability

*   Implement a health check endpoint that provides real-time metrics on ingestion rate, processing lag, and error rates.
*   Use `tracing` for structured logging.

## 7. Gemini Model Interaction

*   **TDD First**: Instruct Gemini to write tests before implementing any logic.
*   **Performance Focus**: When prompting for code, emphasize the need for performance and efficiency.
*   **Rust 2024 Idioms**: Ensure that generated code follows the latest Rust idioms and best practices.