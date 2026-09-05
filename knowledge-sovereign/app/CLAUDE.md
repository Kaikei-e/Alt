# knowledge-sovereign

Knowledge Sovereign service — durable knowledge state の single owner。
Alt の knowledge_events, projections, curation state, retention, export の authority。

## Commands

```bash
# Test (TDD first)
go test ./...

# Build
go build ./...

# Run locally (host port and user per compose/sovereign.yaml)
DATABASE_URL=postgres://sovereign:password@localhost:5438/knowledge_sovereign go run main.go
```

## Clean Architecture

```
Handler (handler/) -> Usecase (usecase/) -> Driver (driver/sovereign_db/)
```

There is no separate `port/`/`gateway/` layer in this service — both `handler/` and `usecase/*` packages import `driver/sovereign_db` directly. ("Port" in comments elsewhere refers to alt-backend's `knowledge_sovereign_port`, the consumer-side interface for calling this service, not a layer inside it.)

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Append-first**: knowledge_events は INSERT-only
3. **Reproject-safe**: Projector はイベントペイロードのみを使う
4. **No shared DB access**: producer は API/event 経由でのみ接続
