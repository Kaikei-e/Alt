# GEMINI.md: Pre-processor Service

This document outlines the essential standards for the `pre-processor` service, adhering to Gemini best practices as of July 2025. This Go-based service is responsible for preprocessing and validating RSS feed data.

## 1. Core Principles

*   **Test-Driven Development (TDD)**: The Red-Green-Refactor cycle is mandatory for all code changes.
*   **Zero Regression Policy**: All existing tests must pass before merging, and no breaking changes are allowed.
*   **Performance**: The service is optimized for high-throughput and low-latency processing.

## 2. Architecture

This service follows a simplified three-layer architecture:

**Handler → Service → Repository**

*   **Handler**: Manages HTTP endpoints.
*   **Service**: Contains the core business logic and is the primary target for testing.
*   **Repository**: Handles data access.

## 3. Development Guidelines

### 3.1. Test-Driven Development (TDD)

1.  **Red**: Write a failing test that fails with assert.Error or assert.ErrorContains.
2.  **Green**: Write the minimal code to pass the test.
3.  **Refactor**: Improve the code while keeping tests green.

*   **Testing Scope**: The `service` layer should have >90% code coverage.
*   **Mocking**: Use `gomock` for mocking external dependencies.

### 3.2. Go 1.23+ Best Practices

*   **Structured Logging**: Use `log/slog` for structured, JSON-formatted logs to integrate with the `rask-log-forwarder`.
*   **Error Handling**: Wrap errors with `fmt.Errorf` to provide context.
*   **HTTP Client**: Use a standard HTTP client with appropriate timeouts and a rate limiter.

## 4. External Dependencies and Rate Limiting

### 4.1. Rate Limiting

*   A minimum 5-second interval between external requests to the same host is mandatory.
*   Implement a rate limiter that waits before making an external request.

### 4.2. Circuit Breaker

*   Implement a circuit breaker pattern to prevent cascading failures from external services.

### 4.3. External Requests in Tests

*   **Unit Tests**: Never make real HTTP requests, database calls, or file I/O in unit tests. Mock all external dependencies.
*   **Integration Tests**: Controlled external access is allowed in integration tests, but they must respect rate limits.

## 5. Logging and Monitoring

### 5.1. Structured Logging

*   All logs must be structured in JSON format to be compatible with the `rask-log-forwarder` sidecar.
*   Enrich logs with a service name, version, and trace ID for better observability.

### 5.2. Rask Log Integration

*   The application does not need to be aware of the Rask logging infrastructure. It only needs to output structured JSON logs to `stdout`/`stderr`.
*   The `rask-log-forwarder` sidecar will automatically collect, parse, and forward the logs.

## 6. Gemini Model Interaction

*   **TDD First**: Instruct Gemini to write comprehensive tests before implementing any logic.
*   **Break Down Tasks**: For complex features, break down the implementation into smaller, verifiable steps.
*   **Safety and Quality**: Use pre-commit hooks and CI quality gates to prevent regressions and ensure code quality.