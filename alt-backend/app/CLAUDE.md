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

## Go 1.23+ Best Practices

### Core Standards
- **Structured Logging:** Always use `log/slog` with context
- **Error Handling:** Wrap errors with `fmt.Errorf("operation failed: %w", err)`
- **Iterators:** Use `iter.Seq` and `iter.Seq2` for custom iterations (Go 1.23+)
- **Memory Management:** Leverage improved timer GC and stack optimizations
- **Dependency Injection:** Constructor pattern with explicit dependencies

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

### Critical TDD Rules
1. **Red-Green-Refactor Cycle:** Always write failing test first. If you are not sure about the test, use `think` or `ultrathink` to think about the test. Test must fail with assert.Error(t, err) or assert.Equal(t, expected, actual)
2. **Test Only:** Usecase and Gateway layers (Driver tests optional)
3. **Mock Dependencies:** Use `gomock` by Uber for external dependencies and use Makefile with make generate-mocks to generate mocks
4. **Coverage Goal:** >80% for tested layers(Usecase and Gateway layers)

### TDD Workflow
```go
// 1. RED: Write failing test
func TestCreateFeed_Success(t *testing.T) {
    // Setup test data and mocks
    // Call method that doesn't exist yet
    // Assert expected behavior
    // Always fail with assert.Error(t, err) or assert.Equal(t, expected, actual)
}

// 2. GREEN: Minimal implementation to pass
func (u *FeedUsecase) CreateFeed(ctx context.Context, req CreateFeedRequest) error {
    return nil // Minimal implementation
}

// 3. REFACTOR: Improve while keeping tests green
```

### Testing Pattern
```go
func TestXxx(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name: "successful case",
            input: InputType{/* valid data */},
            want: OutputType{/* expected result */},
            wantErr: false,
        },
        {
            name: "validation error",
            input: InputType{/* invalid data */},
            want: OutputType{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

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