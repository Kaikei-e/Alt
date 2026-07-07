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
- Never expose internal errors to API clients — log the real error, return a safe message plus an `error_id` for log correlation. Beware `connect.NewError(code, err)`: the wrapped `err.Error()` is sent to the client verbatim (ADR-000054, ADR-000055)
- Let external-service failures cross layers as typed errors (`ExternalHTTPError{StatusCode}`, `ErrTokenUnavailable`), never as opaque `fmt.Errorf` strings — untyped errors collapse to a generic 500 and cannot be branched on in handlers, tests, or alerts (ADR-000313, ADR-000895, PM-2026-043)

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
- Collapse identical concurrent requests with `golang.org/x/sync/singleflight` — a frontend request storm otherwise multiplies straight into the backend (ADR-000320)
- Never upgrade an `RWMutex` read lock to a write lock (`RLock` → check → `Lock`) — the gap between the two is a TOCTOU race the race detector cannot see; use a single `sync.Mutex` critical section instead (ADR-000718)
- Batch loops must check `ctx.Err()` between items and break early — after SIGTERM, a loop that ignores cancellation churns through every remaining item, emitting a cascade of identical errors in the same second (ADR-000147)

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

### Shutdown Ordering for Buffered Pipelines

Services that buffer work (batch writers, stream consumers, event emitters) must shut down in this order — getting it wrong silently drops the final batch:

1. **Stop intake first** (`srv.Shutdown`, stop the XREADGROUP loop)
2. **Flush buffers BEFORE cancelling contexts** — a flush called after `cancel()` fails immediately with `context.Canceled`
3. Ack/commit the flushed work
4. `cancel()` background contexts, then `wg.Wait()` with a bounded deadline

```go
// ❌ cancel() first — final flush always fails with context.Canceled
cancel()
consumer.Stop() // flush inside Stop() gets a dead context

// ✅ flush with a fresh bounded context, then cancel
flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
defer flushCancel()
consumer.Flush(flushCtx)
cancel()
wg.Wait()
```

Every component with a `Stop()`/`Flush()` method must actually be called from the shutdown path — a `Stop()` that exists but is never invoked from `main` is unwired (CLAUDE.md Rule 8).

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
    Addr:              ":8400",
    Handler:           handler,
    ReadHeaderTimeout: 5 * time.Second, // targeted Slowloris defense
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      30 * time.Second,
    IdleTimeout:       60 * time.Second,
    MaxHeaderBytes:    1 << 20, // 1 MB
}
```

Timeout fields default to zero = **no timeout**: a bare `http.ListenAndServe` or an `http.Server` without these fields leaks connections to slow/vanished clients (Slowloris) until fd exhaustion. Every service must construct `http.Server` explicitly with all four timeouts ([Cloudflare guide](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)).

Exception: streaming endpoints (SSE, Connect-RPC streams). `WriteTimeout` applies to the **entire response**, so a 30s value kills every stream after 30 seconds regardless of activity — serve streams from a server with `WriteTimeout: 0` and bound lifetime with per-request context deadlines instead (PM-2026-004).

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
- Begin every transaction with an unconditional `defer tx.Rollback(ctx)` — rollback after a successful commit is a no-op (`pgx.ErrTxClosed`, ignorable via `errors.Is`); conditional defers break under `:=` variable shadowing and leak open transactions (ADR-000328)
- Every UPDATE that assumes the row exists must check `rows_affected == 0` and return an error — a 0-row UPDATE is a silent success that leaves state inconsistent. Do not "fix" it by switching to an UPSERT that implicitly creates the row; that hides the upstream bug (ADR-000113, ADR-000538)

### JSONB Parameters

- The correct Go type for a JSONB parameter depends on the connection path: over **simple protocol through PgBouncer**, `[]byte` is encoded as `bytea` and fails with SQLSTATE 22P02 — pass `string(jsonBytes)`; over a **direct pgx connection**, pass `[]byte`. Follow the service's existing driver helper instead of deciding per call site (ADR-000417, ADR-000470, ADR-000577)
- A nil `json.RawMessage` is sent as an **explicit SQL NULL** — column DEFAULTs only fire when the column is omitted, so `NOT NULL DEFAULT '{}'` still rejects it. Normalize nil/empty payloads to `[]byte("{}")` in a shared driver helper, not ad hoc at each call site (PM-2026-040)

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

## 8. Redis Streams

- Use `go-redis/v9` — parse URL with `redis.ParseURL()`
- Always `Ping` after connecting to verify the connection
- Use `XReadGroup` with consumer groups for stream consumption
- **XACK only after the side effect is durable** (DB commit / index write confirmed) — never on receipt, never on "queued into an in-memory buffer". Acking a message that only reached a buffer makes the pipeline at-most-once: a failed flush silently loses acked events
- **XAUTOCLAIM loop is mandatory**, not optional: without it, entries pending for a crashed consumer stay in the PEL forever and are never redelivered. Configuring `ClaimIdleTime` without running the reclaim loop does nothing
- **Dead-letter via delivery count**: DLQ conditions like `RetryCount > 5` only ever fire if redelivery actually happens (i.e. the XAUTOCLAIM loop above runs). Route poison messages to a dead-letter stream, then XACK them
- Name consumers with `hostname-pid` for traceability

### Stream Consumer Pattern

```go
func (c *Consumer) Run(ctx context.Context) {
    go c.reclaimLoop(ctx) // mandatory companion to XReadGroup

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
            select {
            case <-ctx.Done(): return
            case <-time.After(1 * time.Second):
            }
            continue
        }

        for _, msg := range results[0].Messages {
            if err := c.handler.Handle(ctx, msg); err != nil {
                continue // don't ACK — will be reclaimed by reclaimLoop
            }
            // Handle returned nil ⇒ the side effect is durable. Only now:
            c.rdb.XAck(ctx, "stream-name", c.group, msg.ID)
        }
    }
}

// reclaimLoop recovers PEL entries from crashed consumers and dead-letters poison messages.
func (c *Consumer) reclaimLoop(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
        }
        start := "0-0"
        for {
            msgs, next, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
                Stream: "stream-name", Group: c.group, Consumer: c.consumerName,
                MinIdle: c.claimIdleTime, // > worst-case processing time
                Start:   start, Count: 50,
            }).Result()
            if err != nil {
                slog.Error("XAUTOCLAIM failed", "error", err)
                break
            }
            for _, msg := range msgs {
                if c.deliveryCount(ctx, msg.ID) > maxDeliveries {
                    c.deadLetter(ctx, msg) // XADD to DLQ stream, then XAck
                    continue
                }
                if err := c.handler.Handle(ctx, msg); err == nil {
                    c.rdb.XAck(ctx, "stream-name", c.group, msg.ID)
                }
            }
            if next == "0-0" { break }
            start = next
        }
    }
}
```

### At-Least-Once ⇒ Idempotent Handlers

- Consumer groups are at-least-once: every handler WILL see redeliveries. Non-idempotent side effects need a dedupe key
- Register the dedupe key **in the same DB transaction** as the business write — a separate transaction leaves a crash window where the event is lost or double-applied
- Projection writes must be absolute upserts (`ON CONFLICT ... DO UPDATE SET col = excluded.col`), never additive merges (`col = col + delta`) which double-count on redelivery

> **Alt:** Provision shared infrastructure such as Redis streams, topics, or consumer groups through setup scripts or infrastructure code, not ad hoc application startup side effects.
>
> Sources: [redis.io Streams](https://redis.io/docs/latest/develop/data-types/streams/), [XAUTOCLAIM](https://redis.io/docs/latest/commands/xautoclaim/), [Idempotent Consumer](https://microservices.io/patterns/communication-style/idempotent-consumer.html)

## 9. Configuration

- Pick one configuration strategy per service and keep it consistent
- Simple services often work best with env vars plus defaults; more complex services may justify structured config plus env/secret expansion
- Use typed structs — never pass raw strings around
- Validate required fields early; fail fast in `Load()`
- **Missing required config = exit non-zero at startup.** Never warn-and-limp: a service that starts with an empty JWT secret (rejecting all requests) or an unset upstream URL (silently no-op'ing all mutations) makes misconfiguration indistinguishable from intentional disablement (ADR-000928)
- **"Disabled" must be an explicit config value** (`FEATURE_X=disabled`), logged at startup as `feature_x_disabled` — never inferred from an unset variable

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
- Compare shared secrets with `crypto/subtle.ConstantTimeCompare` — never `==` or `bytes.Equal`. Fail closed when the configured secret is empty; accepting all requests on a missing secret turns a deployment mistake into an auth bypass (ADR-000717)

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
- `http.Client.Timeout` is a **hard ceiling** that covers reading the full response body and always wins over a longer context deadline — a comment saying "the real timeout is controlled by context" is a lie when a client Timeout is set. Streaming clients need `Timeout: 0` with lifetime bounded by the request context; keep unary abuse guards and streaming caps on separate clients (ADR-000146, ADR-000478, ADR-000553)
- Never set `Accept-Encoding` manually — the Transport only transparently decompresses gzip when it added the header itself, so a manual value delivers raw gzip bytes to your parser/decoder. If you must set it, you own decompression; reject unexpected `Content-Encoding` values at the boundary and include magic bytes in decode-failure logs (ADR-000084, ADR-000702, PM-2026-022)
- Use exponential backoff **with jitter** for retryable errors — un-jittered backoff synchronizes retry storms across instances ([AWS](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/))
- Classify errors before retrying — don't retry `400 Bad Request`
- Always use `http.NewRequestWithContext` to propagate cancellation
- **Never bare `time.Sleep` in a retry/wait loop** — it cannot be cancelled. Always `select` on the timer vs `ctx.Done()` as below

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

## 16. UTF-8-Safe Strings

- A Go string is a byte slice: `s[:n]` slices **bytes** and can split a multi-byte UTF-8 sequence, producing invalid output (Japanese text is the common casualty in Alt)
- Truncate at rune boundaries: `string([]rune(s)[:n])` for short strings, or walk with `utf8.DecodeRuneInString` for hot paths
- protobuf3 `string` fields must be valid UTF-8 — serialization fails otherwise. Treat the point just before serialization as the trust boundary and sanitize **every** string field, including DB-sourced values; one missed metadata field made the terminal `done` event unsendable and hung the UI forever (ADR-000596, PM-2026-009)
- Compare lengths in one unit: Go `len()` counts **bytes**, PostgreSQL `LENGTH()` counts **characters** — for Japanese text (3 bytes/char) a cross-system length guard silently never fires. Use `OCTET_LENGTH()` when comparing against Go `len()` (ADR-000548)

```go
// ❌ splits multi-byte characters
title := s[:80]

// ✅ rune-boundary truncation
func truncateRunes(s string, n int) string {
    if utf8.RuneCountInString(s) <= n { return s }
    return string([]rune(s)[:n])
}
```

## 17. Stdlib & Encoding Pitfalls

- Passing an untyped int where `time.Duration` is expected compiles but means **nanoseconds**: `15 * 1000` is 15µs, not 15s — a millionfold error that still "works" while burning CPU, so it evades review and testing. Always multiply by a unit constant: `15 * time.Second` (ADR-000322)
- A nil slice marshals to JSON `null`, not `[]` — `null` slips through `'[]'::jsonb` guards and can overwrite existing data. Normalize nil to an empty slice before marshaling at the boundary (ADR-000529)
- `net/url` preserves trailing slashes — `Parse`/`String` never canonicalize them away. URL normalization needs an explicit trim applied at **every write path**, not just at read time; fixing one ingestion route and missing another reintroduces duplicates (ADR-000052)

---

## References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [Go Proverbs](https://go-proverbs.github.io/)
- [Standard library `net/http` patterns (Go 1.22+)](https://go.dev/blog/routing-enhancements)
