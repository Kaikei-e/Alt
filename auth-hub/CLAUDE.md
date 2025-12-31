# auth-hub/CLAUDE.md

## Overview

Identity-Aware Proxy (IAP) service for the Alt platform. Bridges Nginx's `auth_request` with Ory Kratos session validation. Built with **Go 1.24+**.

> For cache TTLs and Nginx integration details, see `docs/auth-hub.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Start service
go run main.go

# Health check
curl http://localhost:8888/health

# Validate session
curl -H "Cookie: ory_kratos_session=<session>" http://localhost:8888/validate
```

## Architecture

```
Browser → Nginx → [auth-hub] → Backend Services
                      ↓
                   Kratos
```

**Flow:**
1. Nginx intercepts via `auth_request`
2. auth-hub validates session with Kratos `/sessions/whoami`
3. auth-hub caches session (5-min TTL)
4. Returns identity headers: `X-Alt-User-Id`, `X-Alt-Tenant-Id`, etc.

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing strategy:
- **Unit**: Mock KratosClient and SessionCache
- **Integration**: Real Kratos instance
- **Table-driven**: Use for multiple scenarios

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Cache TTL**: 5 minutes (configurable via `CACHE_TTL`)
3. **Never Log Secrets**: Session tokens must never appear in logs
4. **Structured Logging**: Use `log/slog` with JSON format
5. **Error Wrapping**: Always use `fmt.Errorf("context: %w", err)`

## Performance Targets

| Metric | Target |
|--------|--------|
| Cache Hit Rate | >90% |
| P50 Latency (cached) | <10ms |
| P50 Latency (uncached) | <50ms |
| Memory Usage | <50MB |

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| High cache miss rate | Check cache TTL, session consistency |
| 401 errors | Verify Kratos reachability, cookie format |
| High latency | Check Kratos response times |
| Memory growth | Verify cache cleanup goroutine |

## Key Config

```bash
KRATOS_URL=http://kratos:4433
PORT=8888
CACHE_TTL=5m
```

## Appendix: References

### Official Documentation
- [Ory Kratos Sessions](https://www.ory.sh/docs/kratos/session-management/overview)
- [Nginx auth_request](http://nginx.org/en/docs/http/ngx_http_auth_request_module.html)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Identity-Aware Proxy Pattern](https://www.ory.sh/docs/kratos/guides/zero-trust-iap-proxy-identity-access-proxy)
