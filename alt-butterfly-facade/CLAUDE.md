# alt-butterfly-facade/CLAUDE.md

## Overview

BFF service between alt-frontend-sv and alt-backend / acolyte-orchestrator. **Go 1.26+**, HTTP/2 (h2c) transparent proxy.

> Details: `docs/services/alt-butterfly-facade.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Build
go build -o alt-butterfly-facade .

# Run (TTS_PROXY is required: 'enabled' or 'disabled')
TTS_PROXY=disabled ./alt-butterfly-facade

# Health check
curl http://localhost:9200/health
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- Use `NewBackendClientWithTransport(url, timeout, streamingTimeout, http.DefaultTransport)` for tests
- `http.DefaultTransport` uses HTTP/1.1 compatible with `httptest.NewServer`
- Production uses HTTP/2 h2c via `NewBackendClient`

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **No Replace Directives**: Define BFF's own types instead
3. **Transparent Proxy**: Forward requests without modification
4. **JWT Validation**: Always validate before forwarding
5. **Logging**: Use `log/slog` with JSON format
6. **TTS Proxy**: `TTS_PROXY` is required (`enabled` or `disabled`; missing/other -> startup failure). When enabled, `TTS_CONNECT_URL` must be `https://` and uses a dedicated HTTP/1.1 mTLS transport (`MTLS_ENFORCE` does not affect TTS).
