# pre-processor-sidecar/CLAUDE.md

## Overview

Scheduler for RSS ingestion via Inoreader API. **Go**, runs as a long-lived Docker Compose service (`docker compose -f compose/workers.yaml up -d pre-processor-sidecar`) with an internal ticker-based scheduler — not a Kubernetes CronJob.

> Details: `docs/services/pre-processor-sidecar.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Run
go run ./cmd
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

Testing time-sensitive logic requires a `Clock` interface:
```go
type Clock interface {
    Now() time.Time
}
```
Inject `Clock` into services for deterministic testing.

## Critical Rules

1. **Mock All External Deps**: OAuth2 provider, API clients, token repos
2. **No Real Network Calls**: Unit tests MUST be isolated
3. **NEVER Log Secrets**: Use sanitized logging for tokens
4. **Single-Flight**: Use `singleflight.Group` for token refresh
