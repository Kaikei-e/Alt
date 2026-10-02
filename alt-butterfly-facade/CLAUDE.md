# alt-butterfly-facade/CLAUDE.md

## Overview

BFF service between alt-frontend-sv and alt-backend / acolyte-orchestrator. **Go 1.27.1**, HTTP/2 (h2c) transparent proxy.

> Details: `docs/services/alt-butterfly-facade.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Build
go build -o alt-butterfly-facade .

# Run (TTS_PROXY is required: 'enabled' or 'disabled'; default port 9200, compose uses 9250)
TTS_PROXY=disabled BFF_PORT=9250 ./alt-butterfly-facade

# Health check
curl http://localhost:9250/health
```

## TDD Workflow

- Use `NewBackendClientWithTransport(url, timeout, streamingTimeout, http.DefaultTransport)` for tests
- `http.DefaultTransport` uses HTTP/1.1 compatible with `httptest.NewServer`
- Production uses HTTP/2 h2c via `NewBackendClient`

## Critical Rules

1. **No Replace Directives**: Define BFF's own types instead
2. **Transparent Proxy**: Forward requests without modification
3. **JWT Validation**: Always validate before forwarding
4. **Logging**: Use `log/slog` with JSON format
5. **TTS Proxy**: `TTS_PROXY` is required (`enabled` or `disabled`; missing/other -> startup failure). When enabled, `TTS_CONNECT_URL` must be `https://` and uses a dedicated HTTP/1.1 mTLS transport (`MTLS_ENFORCE` does not affect TTS).
