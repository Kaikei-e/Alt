# sidecar-proxy/CLAUDE.md

## Overview

Lightweight Go HTTP proxy deployed alongside alt-backend. Centralizes outbound HTTP policy (timeouts, TLS, allowlist) for consistent egress control.

> For deployment details and config options, see proxy-related sections in `docs/services/alt-backend.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Health check
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Core Responsibilities

- **Policy Enforcement**: HTTPS-only, host allowlisting
- **Connection Management**: Shared timeouts, retries, exponential backoff
- **Header Manipulation**: Trace IDs, user agents
- **Observability**: Structured logs (`slog`), metrics

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing a proxy requires three components:
1. **Mock Client**: Simulates incoming requests
2. **Proxy Handler**: The `httputil.ReverseProxy` being tested
3. **Mock Destination**: `httptest.Server` as upstream service

Key testing scenarios:
- Header manipulation (hop-by-hop stripping, X-Forwarded-For)
- Upstream failure handling
- Timeout behavior
- Body streaming for large payloads

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Use `httptest`**: All tests use `httptest.NewServer` and `httptest.NewRequest`
3. **No Real Network Calls**: Unit tests must be isolated
4. **Structured Logging**: Use `log/slog` for all operations

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Upstream unavailable | Test with closed `httptest.Server` |
| Headers not forwarded | Check hop-by-hop header handling |
| Timeout issues | Verify context deadline propagation |

## Appendix: References

### Official Documentation
- [Go httptest Package](https://pkg.go.dev/net/http/httptest/)
- [httputil.ReverseProxy](https://pkg.go.dev/net/http/httputil#ReverseProxy)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Testing HTTP Handlers in Go](https://golang.cafe/blog/how-to-test-http-handlers-in-go.html)
