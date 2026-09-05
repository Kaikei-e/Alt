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

mq-hub does not interpret `EventType` — it is an opaque, caller-chosen string on a generic
`Publish(stream, event)`. `domain/event.go` still declares `TagsGenerated` as a constant, but no
producer in the repo publishes it today; treat it as dead vocabulary, not a live event. What
actually flows, by stream:

| Event | Producer | Stream | Consumers |
|-------|----------|--------|-----------|
| ArticleCreated / ArticleUpdated | alt-data-hub (`cmd/datahub`, `DataHubService.CreateArticle`, ADR-000954) | `alt:events:articles` | pre-processor, search-indexer, tag-generator (own consumer groups; tag-generator's here is `tag-generator-group`, separate from `tag-generator-tags-group` below) |
| SummarizeRequested | alt-backend — client method exists (`shared/driver/mqhub_connect/client.go`) but no caller publishes it | `alt:events:articles` | pre-processor has a handler, but nothing triggers it today |
| ArticleSummarized | pre-processor — client method exists but has no caller and no DI wiring either | `alt:events:summaries` | none — no consumer anywhere in the repo reads this stream |
| IndexArticle | alt-backend — client method exists but no caller publishes it | `alt:events:index` | search-indexer has an `IndexArticle` case in `consumer/event_handler.go`, but its default consumer only subscribes to `alt:events:articles`, so the case goes unused |
| TagGenerationRequested / TagGenerationCompleted | mq-hub / tag-generator | `alt:events:tags` / `alt:replies:tags:<correlation_id>` | synchronous request-reply for `GenerateTagsForArticle`, consumed by tag-generator's dedicated `tag-generator-tags-group`; timeout defaults to 60s (`DefaultTagGenerationTimeoutMs`), caller-supplied values clamp at 120s |

`alt:events:articles`'s payload is thin only from the consumer side (ADR-000953 stage 3) — mq-hub's
provider-side Pact fixtures must not reintroduce `content`/`tags` fields. The producer
(alt-data-hub) has not completed stage 4: it still embeds `content`/`tags` on the wire today, so
the stream's per-entry byte size has not actually shrunk — treat that as an open gap, not history.

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Usecase**: Mock StreamPort, test business logic
- **Driver**: Use miniredis for unit tests
- **Contract**: Pact CDC tests under the `contract` build tag (`go test -tags=contract ./...`)

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Event Validation**: Always validate before publishing
3. **Idempotency**: Design for at-least-once delivery — dedupe key in the same transaction as the business write; absolute upserts, no additive merges
4. **mq-hub itself never calls `XREADGROUP`**: it only publishes (`XAdd`), creates consumer groups (`XGroupCreateMkStream`), runs the periodic `XTrim` backstop, and `XRead`s its own private reply streams for `GenerateTagsForArticle`. The ACK-after-durable-write / XAUTOCLAIM-reclaim-loop rules in `.claude/rules/event-stream-consumer.md` bind the downstream consumers (pre-processor, search-indexer, tag-generator), not this service — read them before touching those services, not as a description of mq-hub's own runtime behavior
5. **Stream retention has two independent backstops**: `STREAM_MAX_LEN` trims on every `XAdd`; `STREAM_HARD_MAX_LEN` + `STREAM_TRIM_INTERVAL_SECONDS` run a periodic `XTRIM` that keeps working even once Redis is at `maxmemory` and `XAdd` (and its trim) is rejected outright
6. **Graceful Shutdown**: stop the HTTP server, then cancel the trim/sweep loops, with a bounded deadline
7. **HTTP server timeouts**: set `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` explicitly

Full checklist: `docs/best_practices/go.md` §8
