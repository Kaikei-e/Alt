# search-indexer/CLAUDE.md

## Overview

Search indexing service with Meilisearch. **Go 1.26+**, Clean Architecture.

> Details: `docs/services/search-indexer.md`

## Commands

```bash
# Test (TDD first)
go test ./...

# Pact CDC tests (consumer/provider contracts)
go test -tags=contract ./...

# Run
go run main.go
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock repository and search engine interfaces
- **Gateway**: Mock Meilisearch driver
- **Driver**: Fake `meilisearch.ServiceManager` (no real Meilisearch instance in the test suite)

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Batch Size**: Process articles in batches of 200
3. **Primary Key**: Meilisearch `id` field is the upsert key (the article's ID, not `article_id`)
4. **Retry Logic**: Implement exponential backoff
5. **ACK after durable write only**: XACK only once a batch flush confirms the Meilisearch write succeeded — never on receipt or buffer-in
6. **XAUTOCLAIM loop mandatory**: `consumer/redis_consumer.go`'s reclaim loop is what actually redelivers a crashed consumer's pending entries; `ClaimIdleTime` alone does nothing without it
7. **Article payloads are thin only, consumer-side ([[000953]])**: this service's own handler reads `article_id` only from `alt:events:articles`. Do not reintroduce a fat-event branch that reads `content`/`tags` from the payload — resolve the body via alt-data-hub (`GetArticleByID`). Note: the producer (alt-data-hub) has not completed ADR-000953 stage 4 — it still embeds `content`/`tags` on the wire today, so this rule binds what this service's handler decodes, not what actually arrives on the stream

Full checklist: `.claude/rules/event-stream-consumer.md`
