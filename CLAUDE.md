# Alt - AI-Augmented RSS Knowledge Platform

## WHAT

Monorepo with 20+ microservices. Docker Compose-first orchestration, TDD-first development.

| Language | Services | Test | Build |
|----------|----------|------|-------|
| Go 1.24+ | alt-backend, auth-hub, pre-processor, search-indexer, mq-hub, altctl | `go test ./...` | `go build ./...` |
| Python 3.11+ | news-creator, tag-generator, metrics, recap-subworker, recap-evaluator | `uv run pytest` | — |
| Rust 1.87+ | rask-log-aggregator, rask-log-forwarder, recap-worker | `cargo test` | `cargo build` |
| TypeScript | alt-frontend, alt-frontend-sv | `pnpm test` / `bun test` | `pnpm build` |
| Deno 2.x | auth-token-manager, alt-perf | `deno test` | — |

Each service has its own `CLAUDE.md` with service-specific guidance. See `docs/services/MICROSERVICES.md` for the full reference.

## WHY

- **TDD-first**: Write failing test → make it pass → refactor. Quality through tests, not hope.
- **Compose-first**: Docker Compose is the single source of truth. No K8s.
- **Clean Architecture**: `Handler → Usecase → Port → Gateway → Driver` in every service.

## HOW

### Running services

```bash
docker compose -f compose/compose.yaml -p alt up -d           # All
docker compose -f compose/compose.yaml -p alt up -d <service> # One
docker compose -f compose/compose.yaml -p alt logs <service> -f
docker compose -f compose/compose.yaml -p alt down
```

Profiles: `db` | `auth` | `core` | `workers` | `ai` | `rag` | `recap` | `logging` | `observability`

### Verifying changes

```bash
curl http://localhost:3000/api/health   # Frontend
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:7700/health       # Meilisearch
```

After code changes to compiled services (Go, Rust, F#, TypeScript), always rebuild:
```bash
docker compose -f compose/compose.yaml up --build -d <service>
```

## Critical Rules

1. **IMPORTANT: TDD First** — Write the failing test BEFORE writing implementation code. RED → GREEN → REFACTOR.
2. **IMPORTANT: Rate Limiting** — 5-second minimum intervals between external API calls.
3. **IMPORTANT: Rebuild compiled services** — Go/Rust/F#/TS changes require `--build`. Without it, old binaries keep running silently.
4. **No Secrets in Code** — Use `.env` and Docker secrets. Never hardcode credentials.
5. **Service-specific rules** — Always check `<service>/CLAUDE.md` before modifying a service.

## Common Pitfalls

| Issue | Fix |
|-------|-----|
| Stack won't start | `docker compose down` then `up -d` |
| Tests failing on mocks | Check mock interfaces match current implementations |
| Rate limit errors from external APIs | Verify 5-second intervals between calls |
| Import cycles (Go) | Check Clean Architecture layer dependencies |
| Changes not taking effect | Forgot `--build` — rebuild the service |