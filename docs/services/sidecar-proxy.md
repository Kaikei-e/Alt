# sidecar-proxy

_Last reviewed: February 28, 2026_

**Location:** `alt-backend/sidecar-proxy/`

## Purpose

Lightweight Go HTTP proxy deployed alongside alt-backend. Centralizes outbound HTTP policy (timeouts, TLS, allowlist) for consistent egress control.

## Core Responsibilities

- **Policy Enforcement**: HTTPS-only, host allowlisting
- **Connection Management**: Shared timeouts, retries, exponential backoff
- **Header Manipulation**: Trace IDs, user agents
- **Observability**: Structured logs (`slog`), metrics

## Testing Patterns

Testing a proxy requires three components:

1. **Mock Client**: Simulates incoming requests
2. **Proxy Handler**: The `httputil.ReverseProxy` being tested
3. **Mock Destination**: `httptest.Server` as upstream service

### Key Testing Scenarios

- Header manipulation (hop-by-hop stripping, X-Forwarded-For)
- Upstream failure handling
- Timeout behavior
- Body streaming for large payloads

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Upstream unavailable | Test with closed `httptest.Server` |
| Headers not forwarded | Check hop-by-hop header handling |
| Timeout issues | Verify context deadline propagation |

## References

### Official Documentation
- [Go httptest Package](https://pkg.go.dev/net/http/httptest/)
- [httputil.ReverseProxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Testing HTTP Handlers in Go](https://golang.cafe/blog/how-to-test-http-handlers-in-go.html)
