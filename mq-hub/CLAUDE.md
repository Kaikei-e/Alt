# mq-hub/CLAUDE.md

## Overview

Message queue hub service for the Alt RSS reader platform. Built with **Go 1.24+** and **Connect-RPC**. Acts as an event broker using Redis 8.4 Streams for event sourcing.

> For stream configuration and consumer group setup, see `docs/mq-hub.md`.

## Quick Start

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate proto code
cd ../proto && buf generate --template buf.gen.mq-hub.yaml

# Start service
go run main.go

# Check health
curl http://localhost:9500/health
```

## Architecture

Five-layer Clean Architecture:

```
Connect Handler → Usecase → Port → Gateway → Driver
```

- **Connect**: Connect-RPC handlers for event publishing
- **Usecase**: Business logic for stream operations
- **Port**: Interface definitions (StreamPort)
- **Gateway**: Anti-corruption layer for Redis
- **Driver**: Redis Streams client implementation

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing layers:
- **Usecase**: Mock StreamPort, test business logic
- **Gateway**: Mock driver, test domain translation
- **Driver**: Use miniredis for unit tests
- **Integration**: Real Redis 8.4 instance

## Event Types

| Event | Producer | Consumers |
|-------|----------|-----------|
| ArticleCreated | alt-backend | pre-processor, search-indexer, tag-generator |
| SummarizeRequested | alt-backend | pre-processor |
| ArticleSummarized | pre-processor | search-indexer |
| TagsGenerated | tag-generator | search-indexer |

## Stream Keys

| Key | Purpose |
|-----|---------|
| `alt:events:articles` | Article lifecycle events |
| `alt:events:summaries` | Summarization events |
| `alt:events:tags` | Tag generation events |

## Redis 8.4 Features

This service leverages Redis 8.4's XREADGROUP CLAIM option:
- Consume idle pending + new messages in one command
- Simplified failure recovery
- 30% throughput improvement

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Event Validation**: Always validate events before publishing
3. **Idempotency**: Design for at-least-once delivery
4. **Structured Logging**: Use `log/slog` for all operations
5. **Graceful Shutdown**: Handle SIGTERM properly

## Key Config

```bash
REDIS_URL=redis://redis-streams:6379
CONNECT_PORT=9500
LOG_LEVEL=info
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Redis connection failed | Check REDIS_URL and network |
| Consumer group exists | BUSYGROUP error is handled gracefully |
| Slow publishing | Check Redis latency, use batch publishing |
| Memory issues | Configure Redis maxmemory policy |

## Appendix: References

### Official Documentation
- [Redis 8.4 Streams](https://redis.io/docs/latest/develop/whats-new/8-4/)
- [Connect-RPC Go](https://connectrpc.com/docs/go/getting-started/)
- [go-redis](https://redis.uptrace.dev/)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Clean Architecture in Go](https://threedots.tech/post/introducing-clean-architecture/)
