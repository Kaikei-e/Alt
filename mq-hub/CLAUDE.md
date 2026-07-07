# mq-hub/CLAUDE.md

## Overview

Event broker using Redis 8.4 Streams. **Go 1.26+**, **Connect-RPC**.

> Details: `docs/services/mq-hub.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Proto generation
cd ../proto && buf generate --template buf.gen.mq-hub.yaml

# Run
go run main.go
```

## Event Types

| Event | Producer | Consumers |
|-------|----------|-----------|
| ArticleCreated | alt-backend | pre-processor, search-indexer |
| SummarizeRequested | alt-backend | pre-processor |
| ArticleSummarized | pre-processor | search-indexer |
| TagsGenerated | tag-generator | search-indexer |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock StreamPort, test business logic
- **Driver**: Use miniredis for unit tests
- **Integration**: Real Redis 8.4 instance

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Event Validation**: Always validate before publishing
3. **Idempotency**: Design for at-least-once delivery — dedupe key in the same transaction as the business write; absolute upserts, no additive merges
4. **ACK after durable write only**: XACK only once the side effect is committed — never on receipt or buffer-in
5. **XAUTOCLAIM loop mandatory**: `ClaimIdleTime` config alone does nothing; without the reclaim loop, crashed-consumer messages stay pending forever and DLQ conditions never fire
6. **Graceful Shutdown**: stop intake → flush buffers → ack → cancel contexts, with a bounded deadline
7. **HTTP server timeouts**: set `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` explicitly

Full checklist: `.claude/rules/event-stream-consumer.md`, `docs/best_practices/go.md` §8
