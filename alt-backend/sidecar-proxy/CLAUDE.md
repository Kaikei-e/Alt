# sidecar-proxy/CLAUDE.md

## Overview

Lightweight HTTP proxy for alt-backend egress control. **Go**.

> Details: `docs/services/sidecar-proxy.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Health check
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

Testing a proxy requires three components:
1. **Mock Client**: Simulates incoming requests
2. **Proxy Handler**: `httputil.ReverseProxy` being tested
3. **Mock Destination**: `httptest.Server` as upstream

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Use `httptest`**: All tests use `httptest.NewServer` and `httptest.NewRequest`
3. **No Real Network Calls**: Unit tests MUST be isolated
4. **Logging**: Use `log/slog` for all operations
