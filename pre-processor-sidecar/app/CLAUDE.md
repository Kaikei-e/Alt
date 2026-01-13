# pre-processor-sidecar/CLAUDE.md

## Overview

Go scheduler orchestrating RSS ingestion via Inoreader API. Runs as Kubernetes CronJob (production) or Deployment (debugging). Manages OAuth2 token rotation and persistence.

> For scheduler status and token rotation details, see `docs/services/pre-processor-sidecar.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Start service
go run main.go
```

## Core Responsibilities

- **Scheduling**: Periodic article fetches and subscription sync
- **Token Management**: OAuth2 token rotation via Kubernetes Secrets
- **Resilience**: Rate limiting, circuit breaking, single-flight concurrency
- **Admin**: Token health endpoints, manual job triggers

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing time-sensitive logic requires a `Clock` interface:

```go
type Clock interface {
    Now() time.Time
}
```

Inject `Clock` into services for deterministic time-based testing.

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Mock All External Deps**: OAuth2 provider, API clients, token repos
3. **No Real Network Calls**: Unit tests must be isolated
4. **Never Log Secrets**: Use sanitized logging for tokens
5. **Single-Flight**: Use `singleflight.Group` for token refresh

## Kubernetes Best Practices

- **`concurrencyPolicy: Forbid`**: Prevent overlapping job instances
- **`startingDeadlineSeconds`**: Avoid delayed job execution
- **Resource Limits**: Always define CPU/memory requests and limits
- **Least Privilege**: Service account only manages its own secrets

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Token expiry in tests | Use mock Clock interface |
| Concurrent refresh calls | Verify singleflight implementation |
| CronJob overlap | Check concurrencyPolicy setting |

## Appendix: References

### Official Documentation
- [singleflight Package](https://pkg.go.dev/golang.org/x/sync/singleflight)
- [OAuth2 for Go](https://pkg.go.dev/golang.org/x/oauth2)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Kubernetes CronJob Best Practices](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs/)
