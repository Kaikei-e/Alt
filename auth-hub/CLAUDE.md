# auth-hub/CLAUDE.md

## About auth-hub

> Need the latest operational profile (cache TTLs, Nginx integration notes)? See `docs/auth-hub.md`.

**auth-hub** is a lightweight authentication proxy service that implements the Identity-Aware Proxy (IAP) pattern for the Alt RSS reader project. It bridges Nginx's `auth_request` module with Ory Kratos session validation, extracting user identity information and forwarding it as HTTP headers to backend services.

**Core Responsibility:** Centralize authentication at the edge, eliminating the need for backend services to call Kratos directly.

---

## Architecture Overview

### Identity-Aware Proxy Pattern

```
Browser → Nginx → [auth-hub] → Backend Services
                      ↓
                   Kratos
```

**Flow:**
1. Nginx intercepts requests via `auth_request` directive
2. auth-hub validates session with Kratos `/sessions/whoami`
3. auth-hub caches session data (TTL: 5 minutes)
4. auth-hub returns identity headers: `X-Alt-User-Id`, `X-Alt-Tenant-Id`, etc.
5. Nginx forwards headers to backend services

**Benefits:**
- ✅ Single source of truth for authentication
- ✅ Backend services remain auth-agnostic
- ✅ Improved performance through caching
- ✅ Follows Ory's recommended patterns

---

## Directory Structure

```
/auth-hub/
├── main.go              # Application entry point
├── handler/             # HTTP handlers
│   ├── validate.go      # Session validation endpoint
│   ├── validate_test.go
│   ├── health.go        # Health check endpoint
│   └── metrics.go       # Prometheus metrics (future)
├── client/              # External service clients
│   ├── kratos_client.go # Kratos API client
│   └── kratos_client_test.go
├── cache/               # Caching layer
│   ├── session_cache.go # In-memory TTL cache
│   └── session_cache_test.go
├── config/              # Configuration management
│   └── config.go        # Environment-based config
├── go.mod
├── go.sum
├── Dockerfile
├── .dockerignore
├── CLAUDE.md           # This file
└── PLAN.md             # Implementation plan
```

---

## Test-Driven Development (TDD)

### TDD Workflow

This service follows strict TDD practices as defined in the root `CLAUDE.md`:

1. **RED:** Write a failing test
2. **GREEN:** Write minimal code to pass
3. **REFACTOR:** Improve code quality

### Testing Strategy

**Unit Tests:**
- Test each layer in isolation using mocks
- Focus on business logic and edge cases
- Use table-driven tests where appropriate

**Integration Tests:**
- Test with actual Kratos instance
- Validate cache behavior
- Test Nginx integration

**Example Test Structure:**
```go
func TestValidateHandler(t *testing.T) {
    tests := []struct {
        name           string
        sessionCookie  string
        mockSetup      func(*MockKratosClient, *cache.SessionCache)
        expectedStatus int
        expectedHeader string
        wantErr        bool
    }{
        {
            name:          "valid session returns 200 with headers",
            sessionCookie: "ory_kratos_session=valid",
            mockSetup: func(m *MockKratosClient, c *cache.SessionCache) {
                m.On("Whoami", mock.Anything).Return(&client.Identity{
                    ID: "user-123",
                }, nil)
            },
            expectedStatus: 200,
            expectedHeader: "user-123",
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

---

## Core Components

### 1. Configuration (`config/config.go`)

**Responsibilities:**
- Load configuration from environment variables
- Provide sensible defaults
- Validate required settings

**Environment Variables:**
```bash
KRATOS_URL=http://kratos:4433  # Kratos internal URL
PORT=8888                       # Service port
CACHE_TTL=5m                    # Session cache TTL
```

**Usage:**
```go
cfg, err := config.Load()
if err != nil {
    log.Fatal(err)
}
```

---

### 2. Session Cache (`cache/session_cache.go`)

**Responsibilities:**
- Store validated sessions in memory
- Expire entries after TTL
- Thread-safe operations

**Key Operations:**
```go
// Set session data
cache.Set(sessionID, userID, tenantID, email)

// Get session data (returns nil if expired)
entry, found := cache.Get(sessionID)

// Automatic cleanup runs every minute
```

**Performance Characteristics:**
- O(1) get/set operations
- Background cleanup goroutine
- Thread-safe with RWMutex

---

### 3. Kratos Client (`client/kratos_client.go`)

**Responsibilities:**
- Call Kratos `/sessions/whoami` endpoint
- Parse identity information
- Handle errors and timeouts

**Usage:**
```go
kratosClient := client.NewKratosClient("http://kratos:4433")
identity, err := kratosClient.Whoami(sessionCookie)
if err != nil {
    // Handle error
}
```

**Response Structure:**
```json
{
  "identity": {
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "traits": {
      "email": "user@example.com"
    }
  }
}
```

---

### 4. Validate Handler (`handler/validate.go`)

**Responsibilities:**
- Extract session cookie from request
- Check cache for existing session
- Call Kratos on cache miss
- Return identity headers

**Request Flow:**
```
1. Extract "ory_kratos_session" cookie
2. Generate session ID hash
3. Check cache
   ├─ HIT: Return cached headers (fast path)
   └─ MISS: Call Kratos → cache → return headers
```

**Response Headers:**
```
X-Alt-User-Id: 497f6eca-6276-4993-bfeb-53cbbbba6f08
X-Alt-Tenant-Id: 497f6eca-6276-4993-bfeb-53cbbbba6f08
X-Alt-User-Email: user@example.com
X-Alt-User-Role: user
```

---

## API Endpoints

### `GET /validate`

**Purpose:** Validate session and return identity headers

**Request:**
```http
GET /validate HTTP/1.1
Cookie: ory_kratos_session=<session-cookie>
```

**Success Response (200 OK):**
```http
HTTP/1.1 200 OK
X-Alt-User-Id: user-123
X-Alt-Tenant-Id: tenant-456
X-Alt-User-Email: user@example.com
X-Alt-User-Role: user
```

**Error Responses:**
- `401 Unauthorized`: Missing or invalid session
- `500 Internal Server Error`: Kratos communication failure

---

### `GET /health`

**Purpose:** Health check for container orchestration

**Success Response (200 OK):**
```json
{
  "status": "healthy",
  "service": "auth-hub"
}
```

---

## Logging

All logs use **structured JSON format** with `log/slog`:

```json
{
  "time": "2025-09-30T12:00:00Z",
  "level": "INFO",
  "msg": "session validated",
  "session_id": "abc123",
  "user_id": "user-456",
  "cache_hit": true,
  "duration_ms": 5
}
```

**Log Levels:**
- **DEBUG:** Cache hits/misses, detailed flow
- **INFO:** Successful operations, startup
- **WARN:** Non-critical issues (e.g., near expiry)
- **ERROR:** Failed operations, Kratos errors

---

## Error Handling

### Error Categories

1. **Client Errors (4xx):**
   - Missing session cookie
   - Invalid cookie format
   - Expired/revoked session

2. **Server Errors (5xx):**
   - Kratos unreachable
   - JSON parsing errors
   - Internal cache errors

### Error Response Format

```go
// Return 401 for authentication failures
if err != nil {
    return echo.NewHTTPError(http.StatusUnauthorized, "session validation failed")
}
```

**Error Wrapping:**
```go
if err != nil {
    return fmt.Errorf("failed to call kratos: %w", err)
}
```

---

## Performance & Caching

### Cache Strategy

**TTL:** 5 minutes (configurable via `CACHE_TTL`)

**Trade-offs:**
- ✅ 90%+ cache hit rate after warmup
- ✅ Sub-10ms response time for cached sessions
- ⚠️ Revoked sessions may be cached up to TTL

**Cache Key:** Session ID extracted from cookie

**Memory Usage:** ~50MB under normal load (10K active sessions)

---

## Security Considerations

### Threat Model

- **Internal Service:** auth-hub should NOT be exposed to public internet
- **Trust Boundary:** Trusts Nginx to forward correct cookies
- **No Secret Storage:** Doesn't store passwords or tokens

### Security Best Practices

1. **Cookie-Only Authentication:**
   - Only processes `ory_kratos_session` cookies
   - Ignores other cookie types

2. **Session Validation:**
   - Always validates against Kratos on cache miss
   - Respects Kratos's session revocation

3. **Header Injection Prevention:**
   - Nginx must strip client-provided identity headers
   - auth-hub headers are authoritative

4. **Rate Limiting:**
   - Consider adding rate limiting for production
   - Prevent cache exhaustion attacks

---

## Deployment

### Docker Configuration

**Port:** 8888 (internal only)

**Health Check:**
```yaml
healthcheck:
  test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8888/health"]
  interval: 10s
  timeout: 3s
  retries: 3
```

**Environment Variables:**
```yaml
environment:
  KRATOS_URL: http://kratos:4433
  PORT: "8888"
  CACHE_TTL: "5m"
```

---

### Nginx Integration

**Configuration Pattern:**
```nginx
location ~ ^/api/backend/(?<rest>.*)$ {
    # Validate session
    auth_request /auth-validate;

    # Capture identity headers
    auth_request_set $user_id $upstream_http_x_alt_user_id;
    auth_request_set $tenant_id $upstream_http_x_alt_tenant_id;

    # Forward to backend
    proxy_set_header X-Alt-User-Id $user_id;
    proxy_set_header X-Alt-Tenant-Id $tenant_id;
    proxy_pass http://backend/$rest$is_args$args;
}

location = /auth-validate {
    internal;
    proxy_pass http://auth-hub:8888/validate;
    proxy_pass_request_body off;
    proxy_set_header Cookie $http_cookie;
}
```

---

## Monitoring

### Key Metrics

- **Request Rate:** Total requests/sec
- **Cache Hit Rate:** % of requests served from cache
- **Latency:** P50, P95, P99 response times
- **Error Rate:** 4xx and 5xx errors/sec
- **Kratos Call Rate:** Requests to Kratos/sec

### Performance Targets

| Metric | Target |
|--------|--------|
| Cache Hit Rate | >90% |
| P50 Latency (cached) | <10ms |
| P50 Latency (uncached) | <50ms |
| P99 Latency | <100ms |
| Throughput | >1000 req/s |
| Memory Usage | <50MB |

---

## Troubleshooting

### Common Issues

**1. High Cache Miss Rate**
- Check cache TTL configuration
- Verify session cookies are consistent
- Monitor Kratos session expiry

**2. 401 Errors**
- Verify Kratos is reachable
- Check session cookie format
- Confirm user is logged in

**3. High Latency**
- Check Kratos response times
- Verify network connectivity
- Consider increasing cache TTL

**4. Memory Growth**
- Check for cache cleanup goroutine
- Verify TTL expiration working
- Monitor for session leaks

### Debug Commands

```bash
# Check health
curl http://localhost:8888/health

# Test validation with session
curl -v -H "Cookie: ory_kratos_session=<session>" \
     http://localhost:8888/validate

# View logs
docker compose logs auth-hub -f

# Check cache stats (future: metrics endpoint)
curl http://localhost:8888/metrics
```

---

## Development Workflow

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test ./handler -v

# Watch mode (requires external tool)
gotestsum --watch
```

### Local Development

```bash
# Start dependencies
docker compose up -d kratos

# Run auth-hub locally
export KRATOS_URL=http://localhost:4433
export PORT=8888
go run main.go

# Test validation
curl -H "Cookie: ory_kratos_session=<session>" \
     http://localhost:8888/validate
```

---

## Future Enhancements

1. **Redis Cache:** Replace in-memory cache for multi-instance deployments
2. **Prometheus Metrics:** Add `/metrics` endpoint
3. **JWT Support:** Accept JWT tokens in addition to cookies
4. **Circuit Breaker:** Add resilience for Kratos failures
5. **Distributed Tracing:** OpenTelemetry integration
6. **Custom Claims:** Extract additional identity traits

---

## References

- **Root CLAUDE.md:** `/home/USER_NAME/Alt/CLAUDE.md`
- **Implementation Plan:** `./PLAN.md`
- [Ory Kratos Sessions](https://www.ory.sh/docs/kratos/session-management/overview)
- [Nginx auth_request](http://nginx.org/en/docs/http/ngx_http_auth_request_module.html)
- [Identity-Aware Proxy Pattern](https://www.ory.sh/docs/kratos/guides/zero-trust-iap-proxy-identity-access-proxy)

---

## Quick Reference

**Start Service:**
```bash
docker compose up -d auth-hub
```

**View Logs:**
```bash
docker compose logs auth-hub -f
```

**Run Tests:**
```bash
go test ./...
```

**Health Check:**
```bash
curl http://localhost:8888/health
```

**Validate Session:**
```bash
curl -H "Cookie: ory_kratos_session=<session>" \
     http://localhost:8888/validate -v
```

---

**Service Version:** 1.0.0
**Go Version:** 1.23+
**Status:** Ready for Phase 2 implementation
