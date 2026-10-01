# sidecar-proxy/CLAUDE.md
> Historical record — compose does not run this proxy. Not maintained.

## Overview

Standalone Go HTTP proxy (its own module, not part of `alt-backend/app`) that resolves an internal
Envoy upstream address to a real hostname for path-based `/proxy/<url>` requests, and converts
`CONNECT` tunnels into the same shape. **Not referenced by any `compose/*.yaml` today** — see the
callout at the top of `docs/services/sidecar-proxy.md` before assuming it runs in the current stack.

> Details: `docs/services/sidecar-proxy.md`

## Commands

```bash
# Test
go test ./...

# Health check (if run standalone)
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## TDD Workflow

This code does not use `httputil.ReverseProxy`. Tests are per-package unit tests, one file per
package (`pkg/config`, `pkg/dns`, `pkg/autolearn`, `pkg/proxy`): construct the real type and call
its methods/functions directly, asserting on returned values and errors. `httptest.NewRequest` /
`NewRecorder` / `NewServer` ARE used wherever a request or an upstream is needed —
`pkg/proxy/tunneling_test.go` stands up an `httptest.NewServer` as the Envoy upstream.

## Critical Rules

1. **No real network calls**: unit tests MUST be isolated; use `httptest` for the upstream when one is needed
2. **Domain allowlist stays anchored**: any new allowlist-matching code must compile patterns as `^...$`, never a substring match (SSRF class, [[000077]] [[000310]])
3. **Logging**: this package uses a plain `log.Logger`, not `log/slog`; match the existing style rather than introducing a second logging convention
