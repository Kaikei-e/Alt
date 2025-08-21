# CLAUDE.md - Pre-processor Service (Essential Standards)
*Version 2.2 - August 2025 - Performance and Resilience Optimized*

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->

## 🎯 Mission Critical Rules

### TDD (Test-Driven Development) - NON-NEGOTIABLE
**The Red-Green-Refactor cycle MUST be followed for ALL code changes.**
1.  **Red**: Write a failing test that defines a single, specific piece of functionality.
2.  **Green**: Write the absolute minimal code required to make the test pass.
3.  **Refactor**: Improve the code's design and readability while ensuring all tests remain green.

### Zero Regression Policy
- ALL existing tests MUST pass before merging.
- NO breaking changes to existing functionality.
- Quality gates are in place to PREVENT degradation.

## 🏗️ Architecture (Simplified 3-Layer)

**Handler → Service → Repository**

- **Handler**: Manages HTTP endpoints. Thin layer for request/response handling.
- **Service**: Contains the core business logic. **This is the primary target for TDD.**
- **Repository**: Handles data access and other external integrations.

## 🔴🟢🔵 TDD Process

### Core Principles
- **Test One Thing**: Each test should focus on a single behavior or scenario.
- **Descriptive Names**: Test names should clearly describe what they are testing and the expected outcome (e.g., `TestProcessFeed_Success`, `TestProcessFeed_InvalidXML`).
- **Isolate Dependencies**: Use mocks for all external dependencies (databases, other services) to ensure tests are fast and reliable.

### TDD Workflow Example

**1. RED: Write a failing test for a new feature.**
```go
func TestProcessFeed_EmptyContent(t *testing.T) {
    // ... setup
    _, err := service.ProcessFeed("") // Test case for empty input
    require.Error(t, err)
    assert.Equal(t, ErrEmptyContent, err)
}
```

**2. GREEN: Write minimal code to pass.**
```go
var ErrEmptyContent = errors.New("content cannot be empty")

func (s *Service) ProcessFeed(input string) (ProcessedFeed, error) {
    if input == "" {
        return ProcessedFeed{}, ErrEmptyContent
    }
    // ... other logic
}
```

**3. REFACTOR: Improve the implementation.**
```go
func (s *Service) ProcessFeed(input string) (ProcessedFeed, error) {
    if strings.TrimSpace(input) == "" { // More robust check
        return ProcessedFeed{}, ErrEmptyContent
    }
    // ...
}
```

##  Resilience Patterns

### Context-Aware Circuit Breaker
For modern microservices, a context-aware circuit breaker is essential to handle cancellations and timeouts correctly. We recommend using a library like `mercari/go-circuitbreaker`.

```go
import "github.com/mercari/go-circuitbreaker"

// Initialize the circuit breaker in your service constructor
func NewMyService() *MyService {
    return &MyService{
        cb: circuitbreaker.New(
            circuitbreaker.WithFailOnContextCancel(true),
            circuitbreaker.WithFailOnContextDeadline(true),
            circuitbreaker.WithHalfOpenMaxSuccesses(5),
            circuitbreaker.WithOpenTimeout(10 * time.Second),
        ),
    }
}

// Use the circuit breaker for external calls
func (s *MyService) MakeExternalCall(ctx context.Context) error {
    _, err := s.cb.Do(ctx, func() (interface{}, error) {
        // Your external call logic here
        resp, err := http.Get("http://example.com")
        return resp, err
    })
    return err
}
```

### External Request Guidelines (MANDATORY)
- **NEVER** make real HTTP requests, database calls, or file I/O in unit tests.
- **ALWAYS** enforce a minimum 5-second interval between requests to the same host.
- **ALWAYS** implement timeouts (30s max) and use a context-aware circuit breaker.
- **ALWAYS** set a descriptive `User-Agent` header.

## 📝 Unified Logging with `slog`

### Structured Logging Best Practices
- **JSON Output**: In production, always use `slog.NewJSONHandler` to ensure logs are machine-readable.
- **Contextual Attributes**: Enrich logs with key-value pairs for traceability (e.g., `trace_id`, `operation`).
- **Child Loggers**: Create component-specific loggers with pre-defined attributes to add consistent context without repetitive code.

### Example: Using a Child Logger
```go
// Create a base logger in your service constructor
func NewService(logger *slog.Logger) *Service {
    return &Service{
        logger: logger.With("component", "FeedProcessorService"),
    }
}

// Use the child logger in your methods
func (s *Service) ProcessFeed(ctx context.Context, feedURL string) error {
    // This logger already has the "component" attribute
    s.logger.Info("Processing feed", "url", feedURL)

    // Create an even more specific logger for this operation
    opLogger := s.logger.With("operation", "ProcessFeed", "feed_url", feedURL)

    if err != nil {
        opLogger.Error("Failed to process feed", "error", err)
        return err
    }

    opLogger.Info("Feed processed successfully")
    return nil
}
```

## 🧪 Testing Standards

- **Service Layer**: Must have >90% test coverage.
- **Repository Layer**: Must have >80% test coverage.
- **Mocking**: Use `gomock` for all external dependencies.
- **Test Fixtures**: Use test fixtures for reusable test data.

## References

- [Go `slog` Best Practices](https://betterstack.com/community/guides/logging/logging-in-go/)
- [Context-Aware Circuit Breaker (`mercari/go-circuitbreaker`)](https://github.com/mercari/go-circuitbreaker)
- [TDD in Go](https://gabrieleromanato.name/test-driven-development-in-go)
