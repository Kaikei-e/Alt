# CLAUDE.md - Pre-processor Service (Essential Standards)
*Version 2.1 - June 2025 - Performance Optimized*

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->

## 🎯 Mission Critical Rules

### TDD (Test-Driven Development) - NON-NEGOTIABLE
**Red-Green-Refactor cycle MUST be followed for ALL code changes**
1. **RED**: Write failing test FIRST
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve while tests stay green

### Zero Regression Policy
- ALL existing tests MUST pass before merge
- NO breaking changes to existing functionality
- Quality gates PREVENT degradation

### HTTP Client Configuration (MANDATORY)
```go
// Standard HTTP client with timeouts and rate limiting
func NewHTTPClient() *http.Client {
    return &http.Client{
        Timeout: 30 * time.Second,
        Transport: &http.Transport{
            DialContext: (&net.Dialer{
                Timeout: 10 * time.Second,
            }).DialContext,
            TLSHandshakeTimeout:   10 * time.Second,
            ResponseHeaderTimeout: 10 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
            MaxIdleConns:          10,
            MaxIdleConnsPerHost:   2,
        },
    }
}

// Rate-limited client wrapper
type RateLimitedClient struct {
    client      *http.Client
    rateLimiter *RateLimiter
    logger      *slog.Logger
}

func (c *RateLimitedClient) Get(url string) (*http.Response, error) {
    c.rateLimiter.Wait() // MANDATORY wait

    c.logger.Info("making external request",
        "url", url,
        "timestamp", time.Now())

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, fmt.Errorf("request creation failed: %w", err)
    }

    // Set proper User-Agent
    req.Header.Set("User-Agent", "pre-processor/1.0 (+https://alt.example.com/bot)")

    return c.client.Do(req)
}
```

### Circuit Breaker Pattern
```go
// Prevent cascading failures from external services
type CircuitBreaker struct {
    failures    int
    lastFailure time.Time
    threshold   int
    timeout     time.Duration
    mu          sync.RWMutex
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    cb.mu.RLock()
    if cb.failures >= cb.threshold {
        if time.Since(cb.lastFailure) < cb.timeout {
            cb.mu.RUnlock()
            return errors.New("circuit breaker open")
        }
    }
    cb.mu.RUnlock()

    err := fn()

    cb.mu.Lock()
    defer cb.mu.Unlock()

    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        return err
    }

    cb.failures = 0
    return nil
}
```

---

## 📋 Tech Stack (MANDATORY)

### Core Dependencies
```go
// Required libraries - DO NOT substitute
"log/slog"                    // Structured logging (Go 1.23+)
"go.uber.org/mock/gomock"     // Mocking framework
"github.com/jackc/pgx/v5"     // PostgreSQL driver
"github.com/labstack/echo/v4" // HTTP framework
```

### Go 1.23+ Features to Use
- Enhanced `slog` with improved performance
- New `slices` and `maps` packages
- Improved `range` over functions
- Enhanced error wrapping

---

## 🏗️ Architecture (Simplified 3-Layer)

```
/pre-processor
├─ handler/     # HTTP endpoints
├─ service/     # Business logic (PRIMARY TEST TARGET)
├─ repository/  # Data access
├─ model/       # Data structures
└─ test/        # Tests & mocks
```

**Layer Dependencies**: Handler → Service → Repository

---

## 🔴🟢🔵 TDD Process

### 1. Write Failing Test (RED)
```go
func TestProcessFeed(t *testing.T) {
    tests := map[string]struct {
        input    string
        expected ProcessedFeed
        wantErr  bool
    }{
        "valid RSS": {
            input: `<?xml version="1.0"?><rss><channel><title>Test</title></channel></rss>`,
            expected: ProcessedFeed{Title: "Test", Status: "processed"},
            wantErr: false,
        },
        "invalid XML": {
            input: "<invalid>",
            wantErr: true,
        },
    }

    for name, tc := range tests {
        t.Run(name, func(t *testing.T) {
            result, err := service.ProcessFeed(tc.input)

            if tc.wantErr {
                require.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tc.expected.Title, result.Title)
            assert.Equal(t, tc.expected.Status, result.Status)
        })
    }
}
```

### 2. Minimal Implementation (GREEN)
```go
func (s *Service) ProcessFeed(input string) (ProcessedFeed, error) {
    if input == "<invalid>" {
        return ProcessedFeed{}, errors.New("invalid XML")
    }

    return ProcessedFeed{
        Title:  "Test",
        Status: "processed",
    }, nil
}
```

### 3. Refactor (BLUE)
```go
func (s *Service) ProcessFeed(input string) (ProcessedFeed, error) {
    logger := s.logger.With("operation", "process_feed")

    feed, err := s.parser.Parse(input)
    if err != nil {
        logger.Error("parsing failed", "error", err)
        return ProcessedFeed{}, fmt.Errorf("parse failed: %w", err)
    }

    result := ProcessedFeed{
        Title:       feed.Title,
        Status:      "processed",
        ProcessedAt: time.Now(),
    }

    logger.Info("feed processed", "title", result.Title)
    return result, nil
}
```

---

## 🌐 External Dependencies & Rate Limiting

### Rate Limiting Rules (MANDATORY)
```go
// Minimum 5 second intervals between external requests
const MinRequestInterval = 5 * time.Second

type RateLimiter struct {
    lastRequest time.Time
    mu          sync.Mutex
}

func (r *RateLimiter) Wait() {
    r.mu.Lock()
    defer r.mu.Unlock()

    elapsed := time.Since(r.lastRequest)
    if elapsed < MinRequestInterval {
        time.Sleep(MinRequestInterval - elapsed)
    }
    r.lastRequest = time.Now()
}

// Usage in HTTP clients
func (c *FeedClient) FetchFeed(url string) (*Feed, error) {
    c.rateLimiter.Wait() // MANDATORY before external requests

    resp, err := c.httpClient.Get(url)
    // ... rest of implementation
}
```

### External Request Guidelines
- **NEVER** make external HTTP requests in unit tests
- **ALWAYS** use 5+ second intervals between requests
- **ALWAYS** implement timeout and retry logic
- **ALWAYS** respect robots.txt and API limits

---

## 🧪 Testing Standards

### Test Categories & Rules

#### Unit Tests (NO External Dependencies)
```go
// ✅ CORRECT: Mock external dependencies
func TestFeedService_ProcessFeed(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockClient := mocks.NewMockFeedClient(ctrl)
    mockClient.EXPECT().
        FetchFeed("http://example.com/feed").
        Return(&Feed{Title: "Test"}, nil)

    service := NewFeedService(mockClient)
    result, err := service.ProcessFeed("http://example.com/feed")

    require.NoError(t, err)
    assert.Equal(t, "Test", result.Title)
}

// ❌ WRONG: Real HTTP request in unit test
func TestFeedService_ProcessFeed_WRONG(t *testing.T) {
    service := NewFeedService(http.DefaultClient) // DON'T DO THIS
    result, err := service.ProcessFeed("http://real-site.com/feed") // NEVER!
}
```

#### Integration Tests (Controlled External Access)
```go
// ✅ Integration tests with rate limiting
func TestFeedIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    client := NewRateLimitedClient(5 * time.Second) // Respect rate limits
    service := NewFeedService(client)

    // Test with controlled, predictable external resource
    result, err := service.ProcessFeed("http://test-feed.example.com/rss")
    require.NoError(t, err)
}
```

### Test Coverage Requirements
- **Service Layer**: 90%+ coverage (PRIMARY FOCUS)
- **Repository Layer**: 80%+ coverage
- **Handler Layer**: 70%+ coverage

### Mock Generation
```go
//go:generate mockgen -source=repository.go -destination=test/mocks/repository_mock.go

// Usage in tests
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockRepo := mocks.NewMockRepository(ctrl)
mockRepo.EXPECT().Save(gomock.Any()).Return(nil).Times(1)

service := NewService(mockRepo, slog.Default())
```

---

## 📝 Code Standards

### File Headers (MANDATORY)
```go
// ABOUTME: This file handles RSS feed data preprocessing and validation
// ABOUTME: It transforms raw feed data into normalized format for the pipeline
```

### Structured Logging Pattern
```go
func (s *Service) ProcessBatch(ctx context.Context, items []Item) error {
    logger := s.logger.With(
        "operation", "process_batch",
        "batch_size", len(items),
        "trace_id", getTraceID(ctx),
    )

    logger.Info("starting batch processing")

    for i, item := range items {
        if err := s.processItem(ctx, item); err != nil {
            logger.Error("item failed",
                "item_id", item.ID,
                "position", i,
                "error", err)
            return fmt.Errorf("batch failed at position %d: %w", i, err)
        }
    }

    logger.Info("batch completed successfully")
    return nil
}
```

### Database Operations with pgx
```go
func (r *Repository) SaveFeed(ctx context.Context, feed Feed) error {
    query := `INSERT INTO feeds (id, title, content) VALUES ($1, $2, $3)`

    _, err := r.db.Exec(ctx, query, feed.ID, feed.Title, feed.Content)
    if err != nil {
        r.logger.Error("save failed", "error", err, "feed_id", feed.ID)
        return fmt.Errorf("database save failed: %w", err)
    }

    return nil
}
```

---

## 🚨 Claude Code Safety Rules

### When Using Claude Code
```bash
# ALWAYS start with tests
claude "Write comprehensive tests for RSS processing FIRST.
Do NOT implement the logic yet.
Ensure tests fail to verify TDD cycle."

# For complex problems
claude "Think harder about edge cases:
- Malformed XML
- Network timeouts
- Memory limits
- Concurrent access
Write tests covering all scenarios."

# Break down tasks
claude "Implement RSS parsing in steps:
1. Add XML validation only
2. Add content extraction
3. Add error handling
Each step must pass existing tests."
```

### Protection Against Code Breakage
```bash
# Before major changes
git add . && git commit -m "Backup before Claude changes"

# Verification workflow
claude "Fix the failing tests, but first:
1. Understand what the tests expect
2. Run tests to see current failures
3. Implement minimal fix
4. Verify no regressions"

# If Claude breaks functionality
git reset --hard HEAD^
claude "Previous change broke tests. Fix ONLY:
- [List failing tests]
Do NOT modify test files."
```

---

## ⚡ Quality Gates

### Pre-Commit Hook
```bash
#!/bin/sh
set -e

echo "🔍 Quality gates..."

# Format & generate
go fmt ./...
go generate ./...

# Static analysis
go vet ./...
golangci-lint run ./...

# Security
gosec ./...

# Tests with race detection
go test -race ./...

# Coverage check
go test -coverprofile=coverage.out ./...
coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
if (( $(echo "$coverage < 90" | bc -l) )); then
    echo "❌ Coverage $coverage% below 90%"
    exit 1
fi

echo "✅ All gates passed!"
```

### Makefile Targets
```makefile
.PHONY: test coverage quality-check

test:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$coverage < 90" | bc) -eq 1 ]; then \
		echo "❌ Coverage $$coverage% below 90%"; exit 1; \
	fi

quality-check:
	go fmt ./...
	go generate ./...
	golangci-lint run ./...
	gosec ./...
	make test coverage
```

---

### Fundamental Development Rules

#### External Dependencies (CRITICAL)
- **UNIT TESTS**: NEVER make real HTTP requests, database calls, or file I/O
- **RATE LIMITING**: Minimum 5 seconds between external requests
- **TIMEOUTS**: All external calls must have timeouts (30s max)
- **CIRCUIT BREAKERS**: Implement for all external services
- **USER AGENT**: Always identify your service in requests
- **RETRIES**: Exponential backoff with jitter

#### Network Safety
```go
// ❌ FORBIDDEN in unit tests
http.Get("http://example.com")           // Real HTTP request
sql.Open("postgres://...")               // Real DB connection
os.Open("/path/to/file")                // Real file access

// ✅ REQUIRED in unit tests
mockClient.EXPECT().Get(gomock.Any())    // Mocked HTTP
mockRepo.EXPECT().Save(gomock.Any())     // Mocked repository
// Use test fixtures for file data
```

## 🔒 Security & Performance

### Input Validation
```go
func (v *Validator) ValidateInput(input string) error {
    if len(input) == 0 {
        return errors.New("input cannot be empty")
    }

    if len(input) > MaxInputSize {
        return fmt.Errorf("input exceeds %d bytes", MaxInputSize)
    }

    // Prevent XML bombs
    if strings.Count(input, "<") > MaxXMLElements {
        return errors.New("too many XML elements")
    }

    return nil
}
```

### Performance Targets
- Process 1000+ items/minute
- Memory usage < 100MB
- Response time < 500ms
- No goroutine leaks

---

## 🚑 Emergency Procedures

### When Tests Fail After Claude Changes
```bash
# 1. Immediate rollback
git reset --hard HEAD^

# 2. Identify issues
go test ./... -v

# 3. Provide specific fix instruction
claude "Fix these failing tests:
[paste test output]

Requirements:
- DO NOT modify test files
- Follow existing patterns
- Add proper error handling
- Use structured logging"
```

### Quality Recovery
```bash
# Emergency quality check
go test ./... || echo "❌ Tests failing"
go vet ./... || echo "❌ Vet issues"
golangci-lint run ./... || echo "❌ Lint issues"

# If quality degraded
make quality-check || {
    echo "🚨 Quality gates failed"
    echo "Run: git reset --hard HEAD~1"
}
```

---

## 📊 Success Criteria

### Definition of Done
- [ ] Tests written BEFORE implementation (TDD)
- [ ] All tests pass (`go test -race ./...`)
- [ ] Coverage ≥ 90% service layer
- [ ] No lint/vet/security issues
- [ ] Proper error handling & logging
- [ ] No breaking changes to existing functionality

### Performance Checklist
- [ ] Benchmark critical paths
- [ ] Profile memory usage
- [ ] Check goroutine lifecycle
- [ ] Validate database connections
- [ ] Monitor response times

---

## 🎭 Domain Understanding Priority

### RSS Processing Domain
**Core Concepts**:
- Feed validation and parsing
- Content transformation
- Batch processing workflows
- Error recovery strategies

**Business Rules**:
- Process feeds reliably without data loss
- Handle malformed inputs gracefully
- Maintain processing history
- Support concurrent processing

### Alt Ecosystem Integration
- **Input**: Raw RSS feeds from crawlers
- **Output**: Normalized data for main backend
- **SLA**: 99.9% availability, <1min processing time
- **Error Handling**: Dead letter queue for failed items

---

**Remember**: Domain understanding drives implementation. TDD ensures quality. Quality gates prevent regression. Simplicity enables maintainability.