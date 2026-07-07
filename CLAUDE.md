# Alt - AI-Augmented RSS Knowledge Platform

## WHAT

Monorepo with 20+ microservices. Docker Compose-first orchestration, TDD-first development.

| Language | Services | Test | Build |
|----------|----------|------|-------|
| Go 1.26+ | alt-backend, auth-hub, pre-processor, search-indexer, mq-hub, altctl | `go test ./...` | `go build ./...` |
| Python 3.14+ | news-creator, tag-generator, metrics, recap-subworker, recap-evaluator | `uv run pytest` | — |
| Rust 1.94+ | rask-log-aggregator, rask-log-forwarder, recap-worker | `cargo test` | `cargo build` |
| TypeScript | alt-frontend-sv | `bun test` | `bun run build` |
| Deno 2.x | auth-token-manager, alt-perf | `deno test` | — |

Each service has its own `CLAUDE.md` with service-specific guidance. See `docs/services/MICROSERVICES.md` for the full reference.

**Wiki entry**: `docs/wiki/HOME.md` — crystallized navigation layer over ADR / runbook / plan. Read this first to get the current map of the system.

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
curl http://localhost/health             # Frontend (via nginx)
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:9250/health       # BFF
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
6. **Reload canonical context before repair PRs** — Before starting a repair / regression-fix PR that touches the Knowledge Loop, Knowledge Home, or any append-first projection path, re-read `plan/knowledge-loop-core-concept.md` and the affected canonical contract via `/plan-context-loader`. Long-running sessions silently lose the invariants ("context rot") and bug fixes regress to single-axis collapses.
7. **Producer wiring PRs require CDC RED first** — Any PR that adds or modifies a cross-service event producer (new event type, new payload field, new RPC) must land a Pact CDC RED test before the producer GREEN. "Proto compiled + E2E green" is not enough — the silent-fallback failure mode (ADR-000928) hides DI/wiring gaps that defensive nil-guards swallow.
8. **No silent fallback for unwired dependencies** — Optional dependencies (DI options, feature flags, future hooks) must surface their wiring state with a loud `*_enabled` / `*_disabled` startup log AND `panic` when the unwired branch is taken inside business code. Defensive `if x == nil { return nil }` in producer / projector / resolver paths is forbidden — it makes "DI forgot to wire" indistinguishable from "intentionally disabled" (PM-2026-045 / ADR-000928 root cause). Details: `.claude/rules/di-wiring.md`.
9. **Fail-fast startup config** — Missing required config (secrets, upstream URLs, auth tokens) = exit non-zero at startup. Never warn-and-limp. "Disabled" must be an explicit config value with a startup log, never inferred from an unset variable.
10. **Stream consumers: ACK after durable write + reclaim loop** — XACK only after the side effect is durable (not on buffer-in). Every XREADGROUP consumer MUST run an XAUTOCLAIM reclaim loop, or crashed-consumer messages are lost forever and DLQ conditions never fire. Details: `.claude/rules/event-stream-consumer.md`.

## Planning with Obsidian

Obsidian vault (docs/) contains 466+ ADRs, plans, runbooks, reviews.
When planning or designing features:

1. Search relevant ADRs via Obsidian MCP (`mcp__obsidian__get_workspace_files`, `mcp__obsidian__view`)
2. Check canonical contracts in `plan/` for the affected domain
3. Read `review/` for known issues and remediation directives
4. Read latest `daily/` notes for current work context

Key documents:
- `plan/IMPL_BASE.md` — Immutable data model design (append-first, versioned artifacts, projections)
- `plan/knowledge-home-phase0-canonical-contract.md` — Knowledge Home canonical contract
- `plan/alt_knowledge_home_phase_plan.md` — Full phase plan (Phase 0-7)
- `review/knowledge-home-phase1-5-remediation-directives-2026-03-18.md` — Remediation directives

## Immutable Data Model Invariants

- **Append-first**: State via events, not mutable flags. `knowledge_events` is INSERT-only.
- **Reproject-safe**: Projectors use event payload only, never latest state queries.
- **Versioned artifacts**: Summaries/tags use `summary_versions`/`tag_set_versions`.
- **Why as first-class**: Every Home item must explain why it was surfaced.
- **Disposable projections**: Read models (`knowledge_home_items`, `today_digest_view`, `recall_candidate_view`) can be rebuilt from the event log.

## Common Pitfalls

| Issue | Fix |
|-------|-----|
| Stack won't start | `docker compose down` then `up -d` |
| Tests failing on mocks | Check mock interfaces match current implementations |
| Rate limit errors from external APIs | Verify 5-second intervals between calls |
| Import cycles (Go) | Check Clean Architecture layer dependencies |
| Changes not taking effect | Forgot `--build` — rebuild the service |
| Planning without ADR context | Run `/plan-context-loader` to load relevant ADRs first |