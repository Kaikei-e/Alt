# GEMINI.md: Project Documentation

This document provides a comprehensive guide to the project, adhering to Gemini best practices as of July 2025. It outlines the architecture, development guidelines, and operational procedures for this mobile-first RSS reader built with a microservice architecture.

## 1. System Architecture

This project follows a containerized, microservices-based architecture orchestrated with Docker Compose. The core design is a five-layer variant of Clean Architecture for the main application.

### 1.1. Core Components

*   **`alt-frontend`**: A Next.js/React application for the user interface.
*   **`alt-backend`**: The primary backend API, built with Go and the Echo framework.
*   **`db`**: A PostgreSQL database for data persistence.
*   **`meilisearch`**: A full-text search engine.

### 1.2. Data Processing & AI Pipeline

*   **`pre-processor`**: A Go service for data preprocessing.
*   **`tag-generator`**: A Python service for ML-based tag generation.
*   **`news-creator`**: An LLM-based service (using Phi4-mini) for content generation.
*   **`search-indexer`**: A Go service for managing search indexes.

### 1.3. Logging Infrastructure

*   **`rask-log-forwarder`**: A Rust-based log forwarder.
*   **`rask-log-aggregator`**: A Rust-based log aggregator.

### 1.4. Architectural Layers (Clean Architecture)

The main application follows this five-layer pattern:

**REST (handler) → Usecase → Port → Gateway (ACL) → Driver**

*   **Gateway**: Acts as an Anti-Corruption Layer (ACL) to isolate the domain from external services.

## 2. Development Guidelines

### 2.1. Test-Driven Development (TDD)

TDD is mandatory for all new features and bug fixes.

1.  **Red**: Write a failing test that defines the desired behavior.
2.  **Green**: Write the minimum amount of code required to make the test pass.
3.  **Refactor**: Improve the code's design and readability while ensuring all tests remain green.

*   **Testing Strategy**: Focus on testing the `usecase` and `gateway` layers. Mock external dependencies using tools like `gomock`.

### 2.2. Coding Standards

*   **Go**: Use `log/slog` for structured logging, follow idiomatic Go patterns, and use `gofmt` and `goimports` for code formatting.
*   **TypeScript**: Use ES6+ module syntax, apply ESLint and Prettier for code quality, and follow React hooks best practices.
*   **Python**: Use Python 3.13+, follow the PEP 8 style guide, and use type hints for clarity.

### 2.3. Domain-Driven Design (DDD)

*   Design microservices around business capabilities.
*   Use bounded contexts to define service boundaries.
*   Minimize inter-service dependencies.

## 3. Operational Procedures

### 3.1. CI/CD

*   Automated testing is run on every commit.
*   Code quality gates enforce linting and code coverage standards.
*   Docker is used for containerization to ensure consistent environments.

### 3.2. Security

*   Manage secrets using environment variables; never hardcode them.
*   Validate all external inputs to prevent injection attacks and other vulnerabilities.

## 4. Gemini Model Interaction

When using Gemini for code generation or other tasks, follow these best practices:

*   **Prompt Design**: Provide clear and specific instructions. For complex tasks, include few-shot examples to guide the model.
*   **Incremental Changes**: Work in small, testable increments. Create detailed implementation plans before writing code.
*   **Verification**: Use the "plan" mode to review changes before applying them. Commit changes frequently with clear, atomic messages.

## 5. Common Patterns

### 5.1. Error Handling (Go)

```go
if err != nil {
    slog.Error("operation failed",
        "error", err,
        "context", contextInfo)
    return fmt.Errorf("operation failed: %w", err)
}
```

### 5.2. Table-Driven Tests (Go)

```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        // test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### 5.3. Dependency Injection (Go)

```go
type Service struct {
    repo Repository
    logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *Service {
    return &Service{
        repo: repo,
        logger: logger,
    }
}
```

## 6. References

*   [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
*   [Effective Go](https://golang.org/doc/effective_go.html)
*   [Domain-Driven Design](https://martinfowler.com/bliki/DomainDrivenDesign.html)
*   [Test-Driven Development](https://martinfowler.com/bliki/TestDrivenDevelopment.html)
*   [Microservices Patterns](https://microservices.io/patterns/index.html)