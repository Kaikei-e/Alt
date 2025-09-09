# CLAUDE.md - The Alt Project

## About This Project

This document provides a high-level overview of the Alt RSS reader project, a mobile-first application built with a modern, performance-oriented microservice architecture. The entire project is developed with a strict **Test-Driven Development (TDD)** first methodology.

For detailed, service-specific documentation, please refer to the `CLAUDE.md` file located in the root directory of each microservice.

## Core Development Principles

Across all services, we adhere to a set of core principles to ensure quality, maintainability, and performance.

### 1. Test-Driven Development (TDD) First

TDD is the foundation of our development process. All new features and bug fixes must begin with a failing test. This ensures that our code is testable by design and that we have a comprehensive test suite.

-   **Go**: We use `gomock` for mocking and `testify` for assertions.
-   **TypeScript/React**: We use `vitest` for unit testing and `React Testing Library` for component testing.
-   **Python**: We use `pytest` with `pytest-mock` for mocking.
-   **Rust**: We use `mockall` for mocking and framework-specific testing libraries like `axum-test`.

### 2. Clean Architecture

Most of our services follow a five-layer variant of Clean Architecture, ensuring a clear separation of concerns and making our services easier to test and maintain.

**Layers**: `REST Handler` → `Usecase` → `Port` → `Gateway (ACL)` → `Driver`

### 3. Performance and Resilience by Design

-   **High-Performance Code**: We use performance-oriented languages and libraries, such as Rust with zero-copy parsing for our logging pipeline.
-   **Resilience Patterns**: We use circuit breakers, rate limiting, and exponential backoff to ensure our services are resilient to failure.

### 4. Secure by Design

-   **OWASP Top 10**: We test for common vulnerabilities, including the OWASP Top 10 for LLM Applications.
-   **Secure Logging**: We have a strict policy of never logging sensitive information, such as tokens or PII.

## Tech Stack Overview

-   **Go**: Backend services (`alt-backend`, `auth-service`, `pre-processor`, `search-indexer`).
-   **TypeScript/Next.js**: Frontend application (`alt-frontend`).
-   **Python**: ML and data services (`tag-generator`, `news-creator`).
-   **Rust**: High-performance infrastructure (`rask-log-aggregator`, `rask-log-forwarder`).
-   **Databases**: PostgreSQL, Meilisearch, ClickHouse.
-   **Deployment**: Docker, Kubernetes, Skaffold.

## Service-Specific Documentation

For detailed information on each service, including its architecture, TDD guidelines, and best practices, please refer to its `CLAUDE.md` file:

-   `alt-backend`: `alt-backend/app/CLAUDE.md`
-   `alt-backend/sidecar-proxy`: `alt-backend/sidecar-proxy/CLAUDE.md`
-   `auth-service`: `auth-service/app/CLAUDE.md`
-   `alt-frontend`: `alt-frontend/CLAUDE.md`
-   `pre-processor`: `pre-processor/app/CLAUDE.md`
-   `pre-processor-sidecar`: `pre-processor-sidecar/app/CLAUDE.md`
-   `search-indexer`: `search-indexer/app/CLAUDE.md`
-   `tag-generator`: `tag-generator/app/CLAUDE.md`
-   `news-creator`: `news-creator/app/CLAUDE.md`
-   `auth-token-manager`: `auth-token-manager/CLAUDE.md`
-   `rask-log-aggregator`: `rask-log-aggregator/app/CLAUDE.md`
-   `rask-log-forwarder`: `rask-log-forwarder/app/CLAUDE.md`
