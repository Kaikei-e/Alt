# pre-processor/CLAUDE.md

## Overview

Feed processing service for the Alt RSS reader platform. Built with **Go 1.24+** and three-layer architecture. Handles article summarization and quality checking with focus on performance and resilience.

> For configuration knobs and News Creator wiring, see `docs/services/pre-processor.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Run with integration tests
go test -tags=integration ./...

# Run with coverage
go test -cover ./...

# Start service
go run main.go
```

## Architecture

Three-layer architecture:

```
Handler → Service → Repository
```

- **Handler**: HTTP endpoints and request/response handling
- **Service**: Core business logic and orchestration
- **Repository**: Data access and external integrations

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

1. **RED**: Write a failing test
2. **GREEN**: Write minimal code to pass
3. **REFACTOR**: Improve quality, keep tests green

Testing layers:
- **Service layer**: Mock repository, test business logic isolation
- **Handler layer**: Use `httptest`, mock service layer
- **Integration**: Real database, mock external APIs

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Rate Limiting**: MUST enforce 5-second minimum for external APIs
3. **Circuit Breakers**: Use `go-circuitbreaker` for external calls
4. **Structured Logging**: Use `log/slog` with component context
5. **Error Wrapping**: Always use `fmt.Errorf("context: %w", err)`
6. **Context Propagation**: Pass `context.Context` with timeouts

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Circuit breaker tripping | Check upstream service health |
| Rate limit errors | Verify 5-second intervals |
| Memory issues | Check batch size configuration |
| Slow processing | Optimize FEED_WORKER_COUNT |

## Key Config

```bash
FEED_WORKER_COUNT=3
BATCH_SIZE=40
NEWS_CREATOR_URL=http://news-creator:11434
```

## Appendix: References

### Official Documentation
- [Go slog Package](https://pkg.go.dev/log/slog)
- [go-circuitbreaker](https://github.com/mercari/go-circuitbreaker)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [slog Best Practices](https://betterstack.com/community/guides/logging/logging-in-go/)

### Architecture
- [Clean Architecture - Uncle Bob](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
