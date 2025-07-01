# GEMINI.md: alt-backend Service

This document provides best practices for the `alt-backend` service, adhering to Gemini standards as of July 2025. This service is the core backend of the RSS reader, built with Go 1.23+, the Echo framework, and Clean Architecture.

## 1. Core Principles

*   **Test-Driven Development (TDD)**: All code changes must begin with a failing test.
*   **Clean Architecture**: Strictly enforce the five-layer architecture and its dependency rules.
*   **Rate Limiting**: A minimum 5-second interval is required for all external API calls to the same host.

## 2. Architecture

### 2.1. Five-Layer Clean Architecture

**REST Handler → Usecase → Port → Gateway (ACL) → Driver**

*   **Dependency Rule**: Inner layers must not depend on outer layers.

### 2.2. Directory Structure

```
/alt-backend/
├─ main.go
├─ rest/          # HTTP handlers
├─ usecase/       # Business logic
├─ port/          # Interface definitions
├─ gateway/       # Anti-corruption layer
├─ driver/        # External integrations
├─ domain/        # Core entities
├─ di/            # Dependency injection
├─ mocks/         # Generated mocks
└─ utils/         # Utility functions
```

## 3. Development Guidelines

### 3.1. Test-Driven Development (TDD)

1.  **Red**: Write a failing test that fails with assert.Error(t, err) or assert.Equal(t, expected, actual)
2.  **Green**: Write the minimal code to pass the test.
3.  **Refactor**: Improve the code while keeping tests green.

*   **Testing Scope**: Focus on testing the `usecase` and `gateway` layers.
*   **Mocking**: Use `gomock` for mocking dependencies. Generate mocks using `make generate-mocks`.
*   **Coverage**: Aim for >80% code coverage in the `usecase` and `gateway` layers.

### 3.2. Go 1.23+ Best Practices

*   **Structured Logging**: Use `log/slog` for all logging, with contextual information.
*   **Error Handling**: Wrap errors with `fmt.Errorf` to provide context.
*   **Dependency Injection**: Use the constructor pattern for explicit dependency injection.

## 4. Rate Limiting and Security

### 4.1. External API Rate Limiting

*   Use the `golang.org/x/time/rate` package to enforce a minimum 5-second interval between requests to the same host.
*   Implement exponential backoff with jitter for retries.

```go
// Rate limiter for external APIs
var feedFetcher = rate.NewLimiter(rate.Every(5*time.Second), 1)

func (g *FeedGateway) FetchFeed(ctx context.Context, url string) error {
    if err := feedFetcher.Wait(ctx); err != nil {
        return fmt.Errorf("rate limit wait failed: %w", err)
    }
    // ... make API call
}
```

### 4.2. Security

*   Validate all external inputs at the REST layer.
*   Use parameterized queries to prevent SQL injection.
*   Manage secrets using environment variables.

## 5. Common Patterns

### 5.1. Service Constructor

```go
type FeedService struct {
    repo     FeedRepository
    fetcher  FeedFetcher
    logger   *slog.Logger
    limiter  *rate.Limiter
}

func NewFeedService(repo FeedRepository, fetcher FeedFetcher, logger *slog.Logger) *FeedService {
    return &FeedService{
        repo:    repo,
        fetcher: fetcher,
        logger:  logger,
        limiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
    }
}
```

### 5.2. Echo Handler

```go
func (h *FeedHandler) CreateFeed(c echo.Context) error {
    var req CreateFeedRequest
    if err := c.Bind(&req); err != nil {
        h.logger.Error("failed to bind request", "error", err)
        return echo.NewHTTPError(http.StatusBadRequest, "Invalid request format")
    }

    result, err := h.usecase.CreateFeed(c.Request().Context(), req)
    if err != nil {
        h.logger.Error("usecase failed", "error", err)
        return echo.NewHTTPError(http.StatusInternalServerError, "Internal server error")
    }

    return c.JSON(http.StatusCreated, result)
}
```

## 6. Gemini Model Interaction

*   **Planning**: Use "think" or "ultrathink" prompts for complex architectural decisions.
*   **TDD**: Explicitly mention "test-driven development" in prompts to ensure the correct workflow.
*   **Incremental Changes**: Work in small, verifiable increments.