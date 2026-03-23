# Go Best Practices — Alt

## 1. Project Structure

- Use `internal/` for packages that are not part of the module's public API — keep exported packages small and deliberate
- Keep packages small and focused: one responsibility per package (`config`, `db`, `api`, `sse`, `stream`)
- Name packages as nouns, not verbs — `scheduler` not `scheduling`
- Keep `main.go` thin: parse config → connect deps → wire handlers → start server → await signal

> **Alt:** Keep `main.go` thin across Alt Go services such as `alt-backend`, `auth-hub`, `pre-processor`, and `search-indexer`. Wire dependencies there; keep business logic in internal packages.

```go
// ✅ main.go skeleton
func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    slog.SetDefault(logger)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    cfg, err := config.Load()
    if err != nil { slog.Error("config", "error", err); os.Exit(1) }

    pool, err := db.Connect(ctx, cfg.DatabaseURL)
    if err != nil { slog.Error("db", "error", err); os.Exit(1) }
    defer pool.Close()

    handler := api.NewRouter(db.NewPgStore(pool))
    srv := &http.Server{Addr: cfg.Addr, Handler: handler}

    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            slog.Error("server failed", "error", err); os.Exit(1)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    cancel()
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()
    srv.Shutdown(shutdownCtx)
}
```

## 2. Error Handling

- Always wrap errors with context using `fmt.Errorf("action: %w", err)`
- Use `errors.Is` / `errors.As` for programmatic checks — never compare `.Error()` strings
- Define sentinel errors for expected conditions; use custom error types for rich context
- Never expose internal errors to API clients — log the real error, return a safe message

```go
// ✅ Wrap with context
pool, err := pgxpool.NewWithConfig(ctx, cfg)
if err != nil {
    return nil, fmt.Errorf("create pool: %w", err)
}

// ❌ No context
return nil, err
```

### Custom Error Types (API Layer)

```go
type AppError struct {
    Status  int    // HTTP status code
    Code    string // machine-readable code
    Message string // human-readable message
    Err     error  // underlying error — never sent to client
}

func (e *AppError) Error() string {
    if e.Err != nil {
        return fmt.Sprintf("%s: %v", e.Message, e.Err)
    }
    return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// Constructors for common cases
func ErrBadRequest(code, message string) *AppError { ... }
func ErrNotFound(code, message string) *AppError   { ... }
func ErrInternal(message string, err error) *AppError { ... }
```

> **Alt:** Keep transport-specific error mapping at the boundary. HTTP handlers, Connect-RPC handlers, and internal use cases should not all share the same wire format.

## 3. Concurrency

- Pass `context.Context` as the first parameter to every function that does I/O
- Use `sync.WaitGroup` to wait for multiple goroutines to finish during shutdown
- Use `errgroup.WithContext` when sibling goroutines should stop after the first failure
- Guard shared state with `sync.Mutex` — prefer locking small critical sections over large ones
- Use buffered channels for fan-out; unbuffered for synchronization

### Graceful Shutdown Pattern

```go
// Start background workers first
var wg sync.WaitGroup
wg.Add(2)
go func() {
    defer wg.Done()
    consumer.Run(ctx)
}()
go func() {
    defer wg.Done()
    scheduler.Run(ctx)
}()

// Then wait for shutdown
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit
slog.Info("shutting down", "signal", sig.String())
cancel()

// HTTP server shutdown with timeout
shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer shutdownCancel()
if err := srv.Shutdown(shutdownCtx); err != nil {
    slog.Error("server shutdown failed", "error", err)
}

wg.Wait()
```

> **Alt:** Services with background workers, schedulers, or stream consumers must ensure every long-lived goroutine exits on `ctx.Done()`.

## 4. Testing

- Prefer stdlib `testing`, table-driven tests, and small hand-written fakes
- If a service already standardizes on helpers such as GoMock, keep usage local and justified instead of mixing styles ad hoc
- Write table-driven tests with `t.Run` subtests
- Use `httptest.NewRequest` + `httptest.NewRecorder` for HTTP handler tests
- Define mock structs that implement store interfaces — keep them in `_test.go` files
- Use `t.Helper()` on test helper functions
- Use `t.Cleanup()` for teardown instead of `defer` in helpers
- Use `t.Setenv()` for environment variable tests (auto-restores)
- Mark integration tests with `//go:build integration` build tag

### Table-Driven Test

```go
func TestParseDuration(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    time.Duration
        wantErr bool
    }{
        {name: "seconds", input: "30s", want: 30 * time.Second},
        {name: "minutes", input: "5m", want: 5 * time.Minute},
        {name: "invalid", input: "nope", wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := parseDuration(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Interface Mock Pattern

```go
// In _test.go — implements db.Store
type mockStore struct {
    projects []db.Project
    err      error
}

func (m *mockStore) ListProjects(ctx context.Context) ([]db.Project, error) {
    return m.projects, m.err
}

func TestListProjects(t *testing.T) {
    store := &mockStore{
        projects: []db.Project{{ID: uuid.New(), Name: "test"}},
    }
    router := NewRouter(store, sse.NewBroker())
    req := httptest.NewRequest("GET", "/api/projects", nil)
    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", w.Code)
    }
}
```

### Integration Test

```go
//go:build integration

func testPool(t *testing.T) *PgStore {
    t.Helper()
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        t.Skip("DATABASE_URL not set")
    }
    pool, err := Connect(ctx, dbURL)
    if err != nil { t.Fatalf("connect: %v", err) }
    t.Cleanup(pool.Close)
    return NewPgStore(pool)
}
```

## 5. Logging

- Use `log/slog` with `slog.NewJSONHandler(os.Stdout, nil)` — no third-party loggers
- Set default logger once in `main.go` via `slog.SetDefault(logger)`
- Use structured key-value pairs, not string interpolation
- Use `slog.Info` for normal operations, `slog.Warn` for recoverable issues, `slog.Error` for failures

```go
// ✅ Structured logging
slog.Info("scan completed", "target", target.Name, "scan_id", scanID)
slog.Error("XREADGROUP failed", "error", err)

// ❌ String interpolation
slog.Info(fmt.Sprintf("scan %s completed for %s", scanID, target.Name))
```

> **Alt:** Emit structured JSON logs to stdout and let Docker Compose or the deployed runtime handle collection and aggregation.

## 6. HTTP Server

- Prefer Go 1.22+ `http.ServeMux` with method-prefixed patterns for new services unless an existing service already standardizes on another router
- Set explicit timeouts: `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`
- Compose middleware as higher-order functions wrapping `http.Handler`
- Apply middleware in reverse execution order (outermost = first to execute)

### Router Setup

```go
func NewRouter(store db.Store, broker *sse.Broker) http.Handler {
    mux := http.NewServeMux()

    mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
    })

    ph := &projectsHandler{store: store}
    th := &targetsHandler{store: store}
    mux.HandleFunc("GET /api/projects", ph.list)
    mux.HandleFunc("GET /api/projects/{id}/targets", th.list) // path params via r.PathValue("id")

    var handler http.Handler = mux
    handler = corsMiddleware(handler)
    handler = loggingMiddleware(handler)
    handler = recoveryMiddleware(handler) // outermost
    return handler
}
```

### Middleware Pattern

```go
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(sw, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", sw.status,
            "duration_ms", time.Since(start).Milliseconds(),
        )
    })
}
```

### Server Configuration

```go
srv := &http.Server{
    Addr:           ":8400",
    Handler:        handler,
    ReadTimeout:    10 * time.Second,
    WriteTimeout:   30 * time.Second,
    IdleTimeout:    60 * time.Second,
    MaxHeaderBytes: 1 << 20, // 1 MB
}
```

### Response Helpers

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
    writeJSON(w, status, map[string]string{"code": code, "message": message})
}
```

## 7. Database (pgx)

- Use `pgxpool` — never raw `pgx.Conn` in server code
- Define a `Store` interface for all queries — inject it into handlers
- Always use parameterized queries (`$1`, `$2`, ...) — never string-concatenate SQL
- Use cursor-based (keyset) pagination, not `OFFSET`
- Wrap multi-step mutations in transactions

### Connection Pool

```go
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
    cfg, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, fmt.Errorf("parse database url: %w", err)
    }
    cfg.MaxConns = 10
    cfg.MinConns = 2
    cfg.MaxConnLifetime = 30 * time.Minute

    pool, err := pgxpool.NewWithConfig(ctx, cfg)
    if err != nil {
        return nil, fmt.Errorf("create pool: %w", err)
    }
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping database: %w", err)
    }
    return pool, nil
}
```

### Store Interface Pattern

```go
type Store interface {
    ListProjects(ctx context.Context) ([]Project, error)
    ListFindings(ctx context.Context, params FindingParams) ([]Finding, bool, error)
    GetFindingDetail(ctx context.Context, id uuid.UUID) (*FindingDetail, error)
}

type PgStore struct { pool *pgxpool.Pool }
func NewPgStore(pool *pgxpool.Pool) *PgStore { return &PgStore{pool: pool} }
```

### Cursor-Based Pagination

```go
// Encode: composite key → base64
func encodeCursor(score float32, id uuid.UUID) string {
    raw := fmt.Sprintf("%v|%s", score, id.String())
    return base64.URLEncoding.EncodeToString([]byte(raw))
}

// SQL: keyset condition
// WHERE (ranking_score, instance_id) < ($1, $2)
// ORDER BY ranking_score DESC, instance_id DESC
// LIMIT $3
```

> **Alt:** If a service models immutable facts or events, keep append-only tables append-only and update only the derived projection tables designed for current state.

## 8. Redis

- Use `go-redis/v9` — parse URL with `redis.ParseURL()`
- Always `Ping` after connecting to verify the connection
- Use `XReadGroup` with consumer groups for stream consumption
- ACK messages only after successful processing — failed messages get redelivered
- Name consumers with `hostname-pid` for traceability

### Stream Consumer Pattern

```go
type Consumer struct {
    rdb          *redis.Client
    group        string
    consumerName string
    handler      Handler
}

func (c *Consumer) Run(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        results, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
            Group:    c.group,
            Consumer: c.consumerName,
            Streams:  []string{"stream-name", ">"},
            Count:    10,
            Block:    5 * time.Second,
        }).Result()
        if err != nil {
            if err == redis.Nil || ctx.Err() != nil { continue }
            slog.Error("XREADGROUP failed", "error", err)
            time.Sleep(1 * time.Second)
            continue
        }

        for _, msg := range results[0].Messages {
            if err := c.handler.Handle(ctx, msg); err != nil {
                continue // don't ACK — will be redelivered
            }
            c.rdb.XAck(ctx, "stream-name", c.group, msg.ID)
        }
    }
}
```

> **Alt:** Provision shared infrastructure such as Redis streams, topics, or consumer groups through setup scripts or infrastructure code, not ad hoc application startup side effects.

## 9. Configuration

- Pick one configuration strategy per service and keep it consistent
- Simple services often work best with env vars plus defaults; more complex services may justify structured config plus env/secret expansion
- Use typed structs — never pass raw strings around
- Validate required fields early; fail fast in `Load()`

### Environment Variables (Simple)

```go
func Load() (*Config, error) {
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        return nil, fmt.Errorf("DATABASE_URL is required")
    }
    redisURL := os.Getenv("REDIS_URL")
    if redisURL == "" {
        redisURL = "redis://127.0.0.1:6379"
    }
    return &Config{DatabaseURL: dbURL, RedisURL: redisURL}, nil
}
```

### Docker Secrets Fallback

```go
func getEnvOrSecret(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    secretPath := "/run/secrets/" + strings.ToLower(key)
    if data, err := os.ReadFile(secretPath); err == nil {
        return strings.TrimSpace(string(data))
    }
    return fallback
}
```

### Custom YAML Duration

```go
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
    var s string
    if err := value.Decode(&s); err != nil { return err }
    dur, err := time.ParseDuration(s)
    if err != nil { return fmt.Errorf("invalid duration %q: %w", s, err) }
    d.Duration = dur
    return nil
}
```

## 10. Security

- Always use parameterized queries (`$1`, `$2`) — never `fmt.Sprintf` into SQL
- Validate and parse user input at the handler boundary (UUIDs, integers, cursors)
- Set `ReadTimeout`, `WriteTimeout`, `MaxHeaderBytes` on `http.Server`
- Never log secrets or full request bodies
- Return generic error messages to clients; log specifics server-side

```go
// ✅ Parameterized
rows, err := pool.Query(ctx, "SELECT * FROM projects WHERE id = $1", id)

// ❌ String interpolation — SQL injection risk
rows, err := pool.Query(ctx, fmt.Sprintf("SELECT * FROM projects WHERE id = '%s'", id))
```

## 11. Linting & Formatting

- Run `go vet ./...` before every commit — catches common mistakes
- Run `gofmt` / `goimports` automatically (editor or pre-commit hook)
- Use `golangci-lint` for extended checks when available

> **Alt:** Verification should be service-local, for example `cd alt-backend/app && go vet ./...` or `cd search-indexer/app && go test ./...`.

## 12. Docker

- Use multi-stage builds: build stage with full SDK, runtime stage with minimal image
- Copy only the compiled binary into the final stage
- Set `EXPOSE` and `HEALTHCHECK` in Dockerfile
- Use `.dockerignore` to exclude test files, docs, and IDE configs

```dockerfile
# Build stage
FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/service ./main.go

# Runtime stage
FROM gcr.io/distroless/static-debian12
COPY --from=build /bin/service /service
EXPOSE 8400
ENTRYPOINT ["/service"]
```

## 13. Performance

- Pre-allocate slices when length is known: `make([]T, 0, n)`
- Use `sync.Pool` for frequently allocated temporary objects
- Write benchmarks with `testing.B` and compare with `benchstat`
- Profile with `net/http/pprof` in dev — never expose in production

```go
func BenchmarkEncodeJSON(b *testing.B) {
    data := buildTestPayload()
    b.ResetTimer()
    for range b.N {
        json.Marshal(data)
    }
}
```

## 14. Server-Sent Events (SSE)

- Check `http.Flusher` support before starting SSE
- Clear write deadline with `http.NewResponseController` for long-lived connections
- Send heartbeat comments (`: heartbeat\n\n`) every 15 seconds to keep connections alive
- Register/unregister clients with a broker for fan-out

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }
    rc := http.NewResponseController(w)
    rc.SetWriteDeadline(time.Time{}) // disable for long-lived connection

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("X-Accel-Buffering", "no")
    w.WriteHeader(http.StatusOK)
    flusher.Flush()

    // ... register client, loop with select on ctx.Done/event channel/heartbeat ticker
}
```

## 15. HTTP Client & Retry

- Set explicit `Timeout` on `http.Client`
- Use exponential backoff for retryable errors (network errors, `unavailable`, `deadline_exceeded`)
- Classify errors before retrying — don't retry `400 Bad Request`
- Always use `http.NewRequestWithContext` to propagate cancellation

```go
func (c *Client) callWithRetry(ctx context.Context, path string, req, resp any) error {
    backoffs := []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}
    var lastErr error
    for attempt := range len(backoffs) + 1 {
        if err := c.doCall(ctx, path, req, resp); err == nil {
            return nil
        } else {
            lastErr = err
            if !isRetryable(err) { return err }
            if attempt < len(backoffs) {
                select {
                case <-ctx.Done(): return ctx.Err()
                case <-time.After(backoffs[attempt]):
                }
            }
        }
    }
    return lastErr
}
```

---

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Go Proverbs](https://go-proverbs.github.io/)
- [Standard library `net/http` patterns (Go 1.22+)](https://go.dev/blog/routing-enhancements)
