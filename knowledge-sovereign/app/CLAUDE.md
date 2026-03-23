# knowledge-sovereign

Knowledge Sovereign service — durable knowledge state の single owner。
Alt の knowledge_events, projections, curation state, retention, export の authority。

## Commands

```bash
# Test (TDD first)
go test ./...

# Build
go build ./...

# Run locally
DATABASE_URL=postgres://alt:password@localhost:5434/knowledge_sovereign go run main.go
```

## Clean Architecture

```
Handler (handler/) -> Usecase -> Port (interfaces) -> Gateway -> Driver (driver/sovereign_db/)
```

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Append-first**: knowledge_events は INSERT-only
3. **Reproject-safe**: Projector はイベントペイロードのみを使う
4. **No shared DB access**: producer は API/event 経由でのみ接続
