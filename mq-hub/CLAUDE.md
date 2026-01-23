# mq-hub/CLAUDE.md

## Overview

Event broker using Redis 8.4 Streams. **Go 1.24+**, **Connect-RPC**.

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
3. **Idempotency**: Design for at-least-once delivery
4. **Graceful Shutdown**: Handle SIGTERM properly
