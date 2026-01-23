# Alt - AI-Augmented RSS Knowledge Platform

Docker Compose-first orchestration with TDD-first development.

## WHAT

### Tech Stack

| Language | Services | Test Command |
|----------|----------|--------------|
| Go 1.24+ | alt-backend, auth-hub, pre-processor, search-indexer, mq-hub, altctl | `go test ./...` |
| Python 3.11+ | news-creator, tag-generator, metrics, recap-subworker, recap-evaluator | `uv run pytest` |
| Rust 1.87+ | rask-log-aggregator, rask-log-forwarder, recap-worker | `cargo test` |
| TypeScript | alt-frontend, alt-frontend-sv | `pnpm test` / `bun test` |
| Deno 2.x | auth-token-manager, alt-perf | `deno test` |

### Project Map

- `compose/` - Docker Compose files (16 profiles)
- `alt-backend/` - Core API (Go/Echo)
- `alt-frontend-sv/` - Primary frontend (SvelteKit)
- `docs/services/MICROSERVICES.md` - Full service reference

## WHY

- **TDD-first**: Quality through test-driven development
- **Compose-first**: Docker Compose as single source of truth (not K8s)

## HOW

### Docker Compose

```bash
docker compose -f compose/compose.yaml -p alt up -d           # Start all
docker compose -f compose/compose.yaml -p alt up -d <service> # Start one
docker compose -f compose/compose.yaml -p alt logs <service> -f
docker compose -f compose/compose.yaml -p alt down
```

Profiles: `db` | `auth` | `core` | `workers` | `ai` | `rag` | `recap` | `logging` | `observability`

### Architecture

```
Handler -> Usecase -> Port -> Gateway -> Driver
```

Each service follows Clean Architecture. See `<service>/CLAUDE.md` for details.

### Health Checks

```bash
curl http://localhost:3000/api/health   # Frontend
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:7700/health       # Meilisearch
```

## Critical Rules

1. **TDD First**: Write failing test before implementation (RED -> GREEN -> REFACTOR)
2. **Compose-First**: Docker Compose is source of truth
3. **Rate Limiting**: 5-second minimum intervals for external APIs
4. **No Secrets in Code**: Use `.env` and Docker secrets
5. **Service Docs**: See `<service>/CLAUDE.md` for service-specific guidance

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Stack won't start | `docker compose down` then `up -d` |
| Tests failing | Check mock interfaces match implementations |
| Rate limit errors | Verify 5-second intervals |
| Import cycles (Go) | Check layer dependencies |
