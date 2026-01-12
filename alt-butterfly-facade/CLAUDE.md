# alt-butterfly-facade/CLAUDE.md

## Overview

Backend for Frontend (BFF) service for the Alt platform. Transparent proxy between `alt-frontend-sv` and `alt-backend` using HTTP/2 (h2c). Built with **Go 1.25+**.

## Quick Start

```bash
# Run tests
go test ./...

# Build
go build -o alt-butterfly-facade .

# Start service (requires env vars)
./alt-butterfly-facade

# Health check
curl http://localhost:9200/health

# Docker healthcheck (distroless)
./alt-butterfly-facade healthcheck
```

## Architecture

```
alt-frontend-sv → [alt-butterfly-facade] → alt-backend
     :4173              :9200                  :9101
```

**Flow:**
1. Frontend sends Connect-RPC request with `X-Alt-Backend-Token` header
2. BFF validates JWT token (issuer: auth-hub, audience: alt-backend)
3. BFF forwards request to alt-backend via HTTP/2 h2c
4. BFF streams response back to frontend

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Transparent HTTP proxy | Avoids proto import issues; no `replace` directives |
| HTTP/2 h2c transport | Required for Connect-RPC streaming |
| JWT passthrough | Validate once at BFF, forward same token to backend |
| No generated Connect clients | Proto types only; handlers do raw HTTP forwarding |

## Directory Structure

```
alt-butterfly-facade/
├── CLAUDE.md
├── Dockerfile
├── go.mod / go.sum
├── main.go
├── config/
│   └── config.go
└── internal/
    ├── client/
    │   └── backend_client.go    # HTTP/2 client to alt-backend
    ├── domain/
    │   └── user_context.go      # BFF's own UserContext (no replace)
    ├── handler/
    │   └── proxy_handler.go     # Transparent proxy handler
    ├── middleware/
    │   └── auth_interceptor.go  # JWT validation
    └── server/
        └── server.go            # HTTP server + h2c setup
```

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing pattern for HTTP/2:
- Use `NewBackendClientWithTransport(url, timeout, streamingTimeout, http.DefaultTransport)` for tests
- `http.DefaultTransport` uses HTTP/1.1 compatible with `httptest.NewServer`
- Production uses HTTP/2 h2c via `NewBackendClient`

## Streaming Procedures

Procedures that use server streaming (handled with extended timeout):

```go
var streamingProcedures = map[string]bool{
    "/alt.feeds.v2.FeedService/StreamFeedStats":              true,
    "/alt.feeds.v2.FeedService/StreamSummarize":              true,
    "/alt.augur.v2.AugurService/StreamChat":                  true,
    "/alt.morning_letter.v2.MorningLetterService/StreamChat": true,
}
```

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **No Replace Directives**: Define BFF's own types instead
3. **Transparent Proxy**: Forward requests without modification
4. **JWT Validation**: Always validate before forwarding
5. **Structured Logging**: Use `log/slog` with JSON format

## Configuration

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `BFF_PORT` | 9200 | Service port |
| `BACKEND_CONNECT_URL` | http://alt-backend:9101 | Backend URL |
| `BACKEND_TOKEN_SECRET_FILE` | - | JWT secret file path |
| `BACKEND_TOKEN_SECRET` | - | JWT secret (fallback) |
| `BACKEND_TOKEN_ISSUER` | auth-hub | Expected JWT issuer |
| `BACKEND_TOKEN_AUDIENCE` | alt-backend | Expected JWT audience |
| `BFF_REQUEST_TIMEOUT` | 30s | Unary request timeout |
| `BFF_STREAMING_TIMEOUT` | 5m | Streaming request timeout |

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Tests fail with "http2: frame too large" | Use `NewBackendClientWithTransport` with `http.DefaultTransport` |
| Proto import errors | Don't use generated connect packages; use transparent proxy |
| Token validation fails | Check issuer, audience, expiration |
| Streaming doesn't work | Ensure h2c handler wraps the mux |

## Performance Targets

| Metric | Target |
|--------|--------|
| P50 Latency (proxy) | <5ms overhead |
| Memory Usage | <128MB |
| Connection pooling | Via http.Client |
