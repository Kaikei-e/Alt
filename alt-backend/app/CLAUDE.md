# alt-backend/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About alt-backend

This is the **core backend service** of the RSS reader microservice architecture, built with **Go 1.23+**, **Echo framework**, and **Clean Architecture** principles. The service follows Test-Driven Development (TDD) and implements a five-layer clean architecture pattern.

**Critical Guidelines:**
- **TDD First:** Always write failing tests BEFORE implementation
- **Quality Over Speed:** Prevent regressions and maintain code quality
- **Rate Limiting:** External API calls must have minimum 5-second intervals
- **Clean Architecture:** Strict layer dependencies and separation of concerns

## Architecture Overview

### Five-Layer Clean Architecture
```
REST Handler → Usecase → Port → Gateway (ACL) → Driver
```

**Layer Dependencies (Dependency Rule):**
- **REST:** HTTP handlers, routing → depends on Usecase
- **Usecase:** Business logic orchestration → depends on Port
- **Port:** Interface definitions (contracts) → depends on Gateway
- **Gateway:** Anti-corruption layer, domain translation → depends on Driver
- **Driver:** External integrations (DB, APIs, etc.) → no dependencies

### Directory Structure
```
/alt-backend/
├─ main.go        # Application entry point
├─ rest/          # HTTP handlers & Echo routers
│  ├─ handler.go
│  └─ schema.go
├─ usecase/       # Business logic orchestration
│  ├─ fetch_feed_usecase/
│  ├─ search_feed_usecase/
│  └─ register_feed_usecase/
├─ port/          # Interface definitions
│  ├─ fetch_feed_port/
│  ├─ feed_search_port/
│  └─ register_feed_port/
├─ gateway/       # Anti-corruption layer
│  ├─ fetch_feed_gateway/
│  ├─ feed_search_gateway/
│  └─ register_feed_gateway/
├─ driver/        # External integrations
│  ├─ alt_db/     # Database drivers
│  ├─ models/     # Data models
│  └─ search_indexer/
├─ domain/        # Core entities & value objects
│  ├─ rss_feed.go
│  ├─ feed_summary.go
│  └─ feed_reading_status.go
├─ di/            # Dependency injection
│  └─ container.go
├─ job/           # Background jobs
├─ mocks/         # Generated mocks (gomock)
├─ utils/         # Cross-cutting concerns
│  ├─ logger/
│  └─ html_parser/
└─ CLAUDE.md      # This file
```

## Go 1.24+ Best Practices

### Core Standards
- **Structured Logging:** Always use `log/slog` with context for machine-readable logs.
- **Error Handling:** Wrap errors with `fmt.Errorf("operation failed: %w", err)` to preserve context.
- **Dependency Injection:** Use the constructor pattern for explicit dependency injection.

### Go 1.24+ Testing Enhancements
- **`testing/synctest`**: Use this experimental package to test concurrent code with a fake clock, making tests more deterministic.
- **`t.Chdir()`**: Use this function to change the working directory for the duration of a test, useful for file-based operations.
- **`testing.B.Loop`**: A faster way to write benchmarks.

### Code Quality
```go
// Structured logging with context
slog.Info("processing request",
    "operation", "create_feed",
    "user_id", userID,
    "feed_url", feedURL)

// Error wrapping
if err != nil {
    return fmt.Errorf("failed to fetch feed %s: %w", url, err)
}

// Rate limiting for external calls
time.Sleep(5 * time.Second) // Minimum interval between API calls
```

### Naming Conventions
- **Variables:** Clear, descriptive names (avoid single letters except indices)
- **Functions:** Verb-noun pattern (`GetUser`, `CreateFeed`)
- **Interfaces:** `-er` suffix (`Fetcher`, `Parser`)
- **Constants:** UPPER_SNAKE_CASE for public, camelCase for private

## Test-Driven Development (TDD)

### The TDD Cycle: Red-Green-Refactor
1.  **Red**: Write a failing test that clearly defines the desired functionality. The test must fail for the expected reason.
2.  **Green**: Write the **absolute minimum** amount of code required to make the test pass. Elegance is not the goal here; correctness is.
3.  **Refactor**: Improve the code's design, readability, and performance without changing its external behavior. All tests must remain green.

### TDD in Clean Architecture
- **Usecase Layer**: This is the primary target for TDD. Mock repository and gateway interfaces to test business logic in complete isolation.
- **Gateway Layer**: Test the gateway's ability to correctly interact with external services by mocking the driver (e.g., a database client or an HTTP client).
- **Handler Layer**: Use `net/http/httptest` to test Echo handlers. Mock the usecase layer to verify that the handler correctly parses requests, calls the usecase, and formats responses.

### Advanced TDD: Echo Handler Workflow
Here is a more detailed workflow for testing an Echo handler:

1.  **RED: Write the failing test.**
    ```go
    func TestCreateUser_Handler(t *testing.T) {
        // 1. Setup: Create a mock usecase
        mockUsecase := new(mocks.UserUsecase)
        // Define expected input and what the mock should return
        userInput := &dto.CreateUserInput{Name: "test"}
        mockUsecase.On("CreateUser", mock.Anything, userInput).Return(nil)

        // 2. Request: Create a new HTTP request with a JSON body
        e := echo.New()
        jsonBody := `{"name":"test"}`
        req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(jsonBody))
        req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
        rec := httptest.NewRecorder()
        c := e.NewContext(req, rec)

        // 3. Execution: Create handler and call it
        h := handler.NewUserHandler(mockUsecase)
        
        // This will fail because the handler doesn't exist yet
        err := h.CreateUser(c)

        // 4. Assertions
        assert.NoError(t, err)
        assert.Equal(t, http.StatusCreated, rec.Code)
        mockUsecase.AssertExpectations(t)
    }
    ```

2.  **GREEN: Write minimal code to pass.**
    - Create the `UserHandler` and the `CreateUser` method.
    - Bind the request body to a DTO.
    - Call the usecase method.
    - Return the appropriate HTTP status.

3.  **REFACTOR: Improve the implementation.**
    - Add validation for the request body.
    - Enhance error handling and logging.
    - Ensure the code is clean and readable.

### Testing Patterns
- **Table-Driven Tests**: Use this idiomatic Go pattern to test multiple scenarios with different inputs and expected outputs.
- **`stretchr/testify`**: Use the `assert` and `require` packages for expressive and readable assertions.
- **`gomock`**: Use `gomock` to generate mocks from interfaces for reliable dependency isolation.

## Rate Limiting & Web Development

### External API Rate Limiting
- **Minimum Interval:** 5 seconds between requests to same host
- **Implementation:** Use `golang.org/x/time/rate` package
- **Retry Logic:** Exponential backoff with jitter

```go
// Rate limiter for external APIs
var feedFetcher = rate.NewLimiter(rate.Every(5*time.Second), 1)

func (g *FeedGateway) FetchFeed(ctx context.Context, url string) error {
    // Wait for rate limit
    if err := feedFetcher.Wait(ctx); err != nil {
        return fmt.Errorf("rate limit wait failed: %w", err)
    }

    // Make API call
    resp, err := g.client.Get(url)
    // ... handle response
}
```

### Security Best Practices
- **Input Validation:** Validate all external inputs at REST layer
- **SQL Injection:** Use parameterized queries only
- **Secrets Management:** Environment variables, never hardcode
- **HTTPS Only:** All external communications

## Claude Code Best Practices

### Effective Prompting
- **Planning Phase:** Use "think" or "ultrathink" for complex architectural decisions
- **TDD Workflow:** Explicitly mention "test-driven development" in prompts
- **Incremental Changes:** Work in small, testable increments
- **Commit Strategy:** Atomic commits with clear messages

### Quality Control
```markdown
# Sample Claude Code Instructions

1. **Before ANY implementation:**
   - Write failing tests first (TDD)
   - Verify current tests pass
   - Plan the minimal change needed

2. **During implementation:**
   - Follow Clean Architecture layer dependencies
   - Add structured logging with slog
   - Implement proper error handling
   - Respect rate limiting for external calls

3. **After implementation:**
   - Verify all tests pass
   - Run linting (gofmt, goimports)
   - Check for regressions
   - Commit with descriptive message
```

### Preventing Regressions
- **Always run tests:** Before and after changes
- **Use pre-commit hooks:** Automated linting and testing
- **Incremental approach:** Small, verifiable changes
- **Review changes:** Use plan mode before auto-accept

## Common Patterns

### Service Constructor
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

### Echo Handler Pattern
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

### Gateway with Rate Limiting
```go
type HTTPFeedGateway struct {
    client  *http.Client
    limiter *rate.Limiter
    logger  *slog.Logger
}

func (g *HTTPFeedGateway) FetchFeed(ctx context.Context, url string) (*domain.Feed, error) {
    // Rate limiting
    if err := g.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limit exceeded: %w", err)
    }

    g.logger.Info("fetching feed", "url", url)

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    resp, err := g.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch feed: %w", err)
    }
    defer resp.Body.Close()

    // Parse and convert to domain entity
    // ...
}
```

## Development Workflow

### For New Features
1. **Understand Requirements:** Analyze business need thoroughly
2. **Write Integration Test:** End-to-end test that fails
3. **TDD Layer by Layer:**
   - REST handler implementation
   - Usecase test → implementation
   - Gateway test → implementation
   - Driver implementation
4. **Refactor:** Improve code quality while keeping tests green
5. **Document:** Update API docs and decisions

### For Bug Fixes
1. **Reproduce:** Write failing test demonstrating the bug
2. **Fix:** Minimal change to make test pass
3. **Verify:** No regression in existing tests
4. **Refactor:** Improve surrounding code if needed

### Code Review Checklist
- [ ] Tests written before implementation (TDD)
- [ ] All tests passing
- [ ] Clean Architecture dependencies respected
- [ ] Rate limiting implemented for external calls
- [ ] Structured logging with context (refer: /alt-backend/app/utils/logger/init.go)
- [ ] Error handling with proper wrapping
- [ ] No hardcoded values
- [ ] `gofmt` and `goimports` applied

## Troubleshooting

### Common Issues
- **Import Cycles:** Check layer dependencies
- **Rate Limit Errors:** Verify 5-second minimum intervals
- **Test Failures:** Check mock interfaces match implementations
- **Performance:** Profile before optimizing

### Debug Tips
- Use structured logging with request IDs
- Write reproducible test cases
- Check service health endpoints
- Monitor rate limiter metrics

## References

- [Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Effective Go](https://golang.org/doc/effective_go.html)
- [Go 1.23 Release Notes](https://tip.golang.org/doc/go1.23)
- [Rate Limiting in Go](https://pkg.go.dev/golang.org/x/time/rate)
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)