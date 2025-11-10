# pre-processor/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About pre-processor

> For a point-in-time state brief covering disabled jobs, News Creator wiring, and config knobs, check `docs/pre-processor.md`.

This is the **pre-processing service** of the Alt RSS reader platform, built with **Go 1.24+** and **Clean Architecture** principles. The service handles feed processing, article summarization, and quality checking with a focus on performance and reliability.

**Critical Guidelines:**
- **TDD First:** Always write failing tests BEFORE implementation
- **Performance:** Optimize for high-throughput processing
- **Resilience:** Implement circuit breakers and retry logic
- **Structured Logging:** Use `log/slog` with context for all operations
- **Rate Limiting:** External API calls must have minimum 5-second intervals

## Architecture Overview

### Three-Layer Architecture
```
Handler → Service → Repository
```

**Layer Responsibilities:**
- **Handler**: HTTP endpoints and request/response handling
- **Service**: Core business logic and orchestration
- **Repository**: Data access and external integrations

### Directory Structure
```
/pre-processor/app/
├─ main.go                    # Application entry point
├─ handler/                   # HTTP handlers
│  ├─ job_handler.go         # Background job handlers
│  ├─ health_handler.go      # Health check handlers
│  └─ summarize_handler.go   # Summarization API handlers
├─ service/                   # Business logic services
│  ├─ feed_processor.go      # Feed processing logic
│  ├─ article_summarizer.go  # Article summarization
│  ├─ quality_checker.go     # Quality checking
│  └─ health_checker.go      # Health monitoring
├─ repository/                # Data access layer
│  ├─ article_repository.go  # Article data access
│  ├─ feed_repository.go     # Feed data access
│  └─ summary_repository.go  # Summary data access
├─ driver/                    # External integrations
│  └─ database.go            # Database connection
├─ utils/                     # Utilities
│  ├─ logger/                # Structured logging
│  └─ http_client.go         # HTTP client utilities
├─ config/                    # Configuration
│  └─ config.go              # Environment configuration
└─ CLAUDE.md                 # This file
```

## TDD and Testing Strategy

### Test-Driven Development (TDD)
All development follows the Red-Green-Refactor cycle:

1. **Red**: Write a failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code quality

### Testing Layers

#### Unit Tests
```go
func TestProcessFeed_EmptyContent(t *testing.T) {
    // Setup
    service := NewFeedProcessorService(mockRepo, mockFetcher, logger)

    // Test
    _, err := service.ProcessFeed("")

    // Assert
    require.Error(t, err)
    assert.Equal(t, ErrEmptyContent, err)
}
```

#### Integration Tests
```go
func TestFeedProcessor_Integration(t *testing.T) {
    // Setup with real database
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)

    service := NewFeedProcessorService(realRepo, mockFetcher, logger)

    // Test
    result, err := service.ProcessFeed("https://example.com/feed")

    // Assert
    require.NoError(t, err)
    assert.NotEmpty(t, result.Articles)
}
```

### Testing Best Practices
- **Mock External Dependencies**: Use mocks for databases and external APIs
- **Table-Driven Tests**: Use for multiple test cases
- **Test Coverage**: Aim for >90% coverage in service layer
- **Performance Tests**: Benchmark critical paths

## Performance and Resilience

### Circuit Breaker Pattern
```go
import "github.com/mercari/go-circuitbreaker"

// Initialize circuit breaker
func NewService() *Service {
    return &Service{
        cb: circuitbreaker.New(
            circuitbreaker.WithFailOnContextCancel(true),
            circuitbreaker.WithFailOnContextDeadline(true),
            circuitbreaker.WithHalfOpenMaxSuccesses(5),
            circuitbreaker.WithOpenTimeout(10 * time.Second),
        ),
    }
}

// Use circuit breaker for external calls
func (s *Service) MakeExternalCall(ctx context.Context) error {
    _, err := s.cb.Do(ctx, func() (interface{}, error) {
        // External call logic
        resp, err := http.Get("http://example.com")
        return resp, err
    })
    return err
}
```

### Rate Limiting
- **External APIs**: Minimum 5-second interval between requests
- **Database**: Use connection pooling and query optimization
- **Memory**: Implement proper garbage collection and memory management

### Error Handling
```go
// Structured error handling
func (s *Service) ProcessFeed(ctx context.Context, url string) error {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    if err := s.validateURL(url); err != nil {
        return fmt.Errorf("invalid URL %s: %w", url, err)
    }

    // Process with proper error wrapping
    if err := s.fetchAndProcess(ctx, url); err != nil {
        return fmt.Errorf("failed to process feed %s: %w", url, err)
    }

    return nil
}
```

## Structured Logging

### Logging Best Practices
```go
// Create component-specific logger
func NewService(logger *slog.Logger) *Service {
    return &Service{
        logger: logger.With("component", "FeedProcessorService"),
    }
}

// Use structured logging with context
func (s *Service) ProcessFeed(ctx context.Context, feedURL string) error {
    s.logger.Info("Processing feed", "url", feedURL)

    opLogger := s.logger.With("operation", "ProcessFeed", "feed_url", feedURL)

    if err := s.fetchFeed(ctx, feedURL); err != nil {
        opLogger.Error("Failed to fetch feed", "error", err)
        return err
    }

    opLogger.Info("Feed processed successfully")
    return nil
}
```

## API Endpoints

### Background Jobs
- **Feed Processing**: Disabled for ethical compliance
- **Summarization**: Processes articles for AI summarization
- **Quality Check**: Validates content quality

### HTTP API
- **POST /api/v1/summarize**: Summarize article content
- **GET /api/v1/health**: Health check endpoint

## Configuration

### Environment Variables
```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=alt_db
DB_PRE_PROCESSOR_USER=pre_processor
DB_PRE_PROCESSOR_PASSWORD=password

# External Services
NEWS_CREATOR_URL=http://news-creator:11434
HTTP_PORT=9200

# Processing
FEED_WORKER_COUNT=3
BATCH_SIZE=40
```

## Development Workflow

### Running Tests
```bash
# Unit tests
go test ./...

# Integration tests
go test -tags=integration ./...

# Coverage
go test -cover ./...

# Benchmarks
go test -bench=. ./...
```

### Running the Service
```bash
# Development
go run main.go

# With Docker
docker build -t pre-processor .
docker run -p 9200:9200 pre-processor
```

## References

- [Go `slog` Best Practices](https://betterstack.com/community/guides/logging/logging-in-go/)
- [Context-Aware Circuit Breaker](https://github.com/mercari/go-circuitbreaker)
- [TDD in Go](https://gabrieleromanato.name/test-driven-development-in-go)
