# alt-backend

Core backend for Alt RSS platform. **Go 1.24+**, **Echo**, Clean Architecture.

> Details: `docs/services/alt-backend.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Coverage
go test -race -cover ./...

# Mocks
make generate-mocks

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock ports, test business logic
- **Gateway**: Mock drivers, test external calls
- **Handler**: Use `httptest`, mock usecases

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: YOU MUST enforce 5-second minimum for external APIs
3. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
4. **Context**: Pass `context.Context` through entire call chain
5. **Logging**: Use `log/slog` with structured context

---

## Clean Architecture

```
Handler (rest/) -> Usecase -> Port (interfaces) -> Gateway -> Driver
```

### 1. Handler Layer (`app/rest/`)

HTTP entrypoint. Request validation, response formatting.

**Files:**
- `app/rest/routes.go` - Route registration
- `app/rest/rest_feeds/fetch.go` - Feed handlers
- `app/rest/recap_handlers.go` - Recap handlers

**Pattern:**
```go
func RestHandleFetchSingleFeed(container *di.ApplicationComponents, cfg *config.Config) echo.HandlerFunc {
    return func(c echo.Context) error {
        feed, err := container.FetchSingleFeedUsecase.Execute(c.Request().Context())
        if err != nil {
            return HandleError(c, err, "fetch_single_feed")
        }
        return c.JSON(http.StatusOK, feed)
    }
}
```

**Can import:** Usecase, Port, DI Container

### 2. Usecase Layer (`app/usecase/`)

Business logic orchestration. No external dependencies.

**Files:**
- `app/usecase/fetch_feed_usecase/single_feed_usecase.go`
- `app/usecase/register_feed_usecase/`
- `app/usecase/fetch_feed_stats_usecase/`

**Pattern:**
```go
type FetchSingleFeedUsecase struct {
    fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedUsecase(port fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedUsecase {
    return &FetchSingleFeedUsecase{fetchSingleFeedPort: port}
}

func (u *FetchSingleFeedUsecase) Execute(ctx context.Context) (*domain.RSSFeed, error) {
    return u.fetchSingleFeedPort.FetchSingleFeed(ctx)
}
```

**Can import:** Port only (+ domain, utils/errors)

### 3. Port Layer (`app/port/`)

Interface definitions (contracts).

**Files:**
- `app/port/fetch_feed_port/fetch_port.go`
- `app/port/register_feed_port/register_port.go`
- `app/port/rate_limiter_port/rate_limiter_port.go`

**Pattern:**
```go
type FetchSingleFeedPort interface {
    FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error)
}

type FetchFeedsPort interface {
    FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error)
    FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error)
    FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error)
}
```

**Can import:** Domain only

### 4. Gateway Layer (`app/gateway/`)

Anti-corruption layer. External service boundary.

**Files:**
- `app/gateway/fetch_feed_gateway/single_feed_gateway.go`
- `app/gateway/register_feed_gateway/`
- `app/gateway/rate_limiter_gateway/`

**Pattern:**
```go
type SingleFeedGateway struct {
    alt_db      *alt_db.AltDBRepository
    rateLimiter *rate_limiter.HostRateLimiter
}

func (g *SingleFeedGateway) FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error) {
    // 1. Fetch from database via driver
    feedURLs, err := g.alt_db.FetchRSSFeedURLs(ctx)
    // 2. Apply rate limiting
    // 3. Call external service
    // 4. Convert to domain model
    return domainFeed, nil
}
```

**Can import:** Port (implements), Driver, Domain

### 5. Driver Layer (`app/driver/`)

Database, external APIs, infrastructure.

**Files:**
- `app/driver/alt_db/repository.go` - PostgreSQL repository
- `app/driver/kratos_client/client.go` - Auth client
- `app/driver/search_indexer/api.go` - Search index

**Pattern:**
```go
type AltDBRepository struct {
    pool PgxIface
}

func (r *AltDBRepository) FetchRSSFeedURLs(ctx context.Context) ([]*url.URL, error) {
    rows, err := r.pool.Query(ctx, "SELECT url FROM rss_feeds")
    // ...
}
```

**Can import:** External libraries only

### 6. Domain Layer (`app/domain/`)

Core business entities.

**Files:**
- `app/domain/feed.go`
- `app/domain/article.go`
- `app/domain/rss_feed.go`

**Can import:** Nothing (pure Go)

## DI Container (`app/di/container.go`)

Wires layers together:

```
Driver -> Gateway (implements Port) -> Usecase -> Handler
```

## Layer Violations (AVOID)

| Violation | Fix |
|-----------|-----|
| Handler -> Driver | Go through Usecase + Gateway |
| Usecase -> external pkg | Create Port interface |
| Gateway -> Usecase | Gateway only implements Port |
| Circular dependency | Extract to new Port |

## References

- [Three Dots Labs - Clean Architecture](https://threedots.tech/post/introducing-clean-architecture/)
- [bxcodec/go-clean-arch](https://github.com/bxcodec/go-clean-arch)
