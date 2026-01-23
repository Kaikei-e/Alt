# pre-processor-sidecar/CLAUDE.md

## Overview

Scheduler for RSS ingestion via Inoreader API. **Go**, K8s CronJob/Deployment.

> Details: `docs/services/pre-processor-sidecar.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Run
go run main.go
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

1. **TDD First**: No implementation without failing tests
2. **Mock All External Deps**: OAuth2 provider, API clients, token repos
3. **No Real Network Calls**: Unit tests MUST be isolated
4. **NEVER Log Secrets**: Use sanitized logging for tokens
5. **Single-Flight**: Use `singleflight.Group` for token refresh
