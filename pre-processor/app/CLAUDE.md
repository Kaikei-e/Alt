# pre-processor/CLAUDE.md

## Overview

Article summarization, Inoreader-to-articles sync, and summary quality-gating service. **Go 1.26+**, `handler` → `service` → `repository`/`driver` layering. (RSS feed *fetching* is not this service's job — see `docs/services/pre-processor.md`; the feed-processing code path here is unwired dead code.)

> Details: `docs/services/pre-processor.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Integration tests
go test -tags=integration ./...

# Coverage
go test -cover ./...

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Service**: Mock repository, test business logic
- **Handler**: Use `httptest`, mock service layer
- **Integration**: Real database, mock external APIs

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: YOU MUST enforce 5-second minimum for external APIs (`utils.HostRateLimiter` / `utils.DefaultHostRateLimiter`)
3. **Retries**: Use `utils/errors.RetryPolicy` + `RetryExecutor.Execute`'s exponential backoff for external calls — there is no circuit-breaker library in this service
4. **Context**: Pass `context.Context` with timeouts
5. **Error Wrapping**: Use `fmt.Errorf("context: %w", err)`
