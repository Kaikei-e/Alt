# Alt - AI-Augmented RSS Knowledge Platform

## WHAT

Monorepo with 20+ microservices (Go, Python, Rust, TypeScript, Deno). Docker Compose-first orchestration, TDD-first development.

Each service has its own `CLAUDE.md` with service-specific guidance. See `docs/services/MICROSERVICES.md` for the full reference.

**alt-backend / alt-harvester / alt-data-hub は 1 ディレクトリ 3 バイナリ** — `alt-backend/app`
という単一 Go モジュール (`module alt`) から `cmd/backend`（ユーザ向け API）/ `cmd/harvester`
（7 定期ジョブ）/ `cmd/datahub`（alt-db の唯一のオーナー、mTLS 専用）の 3 本を切り出し、
同じ `alt-backend/Dockerfile.backend` を `--build-arg BINARY=backend|harvester|datahub` で
3 イメージにビルドする。3 つとも `cd alt-backend/app && go test ./...` で一括してテストする。
根拠と責務分担は `docs/ADR/000954.md`。

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

Compose stacks are wired via **`include:`**, not Compose profiles — see [`compose/compose.yaml`](compose/compose.yaml).

### Verifying changes

```bash
curl http://localhost/health             # Frontend (via nginx)
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:9250/health       # BFF
curl http://localhost:7700/health       # Meilisearch
```

## Critical Rules

1. **IMPORTANT: TDD First** — Write the failing test BEFORE writing implementation code. RED → GREEN → REFACTOR.
2. **IMPORTANT: Rate Limiting** — 5-second minimum intervals between external API calls.
3. **IMPORTANT: Rebuild compiled services** — Go/Rust/TS changes require `--build`. Without it, old binaries keep running silently.
4. **No Secrets in Code** — Use `.env` and Docker secrets. Never hardcode credentials.
5. **Service-specific rules** — Always check `<service>/CLAUDE.md` before modifying a service.
6. **Reload canonical context before repair PRs** — Before starting a repair / regression-fix PR that touches the Knowledge Trail, Knowledge Home, or any append-first projection path, re-read `docs/plan/knowledge-trail-core-concept.md` and the affected canonical contract via `/plan-context-loader`. Long-running sessions silently lose the invariants ("context rot") and bug fixes regress to single-axis collapses. Knowledge Loop contracts are historical only ([[000940]]).
7. **Producer wiring PRs require CDC RED first** — Any PR that adds or modifies a cross-service event producer (new event type, new payload field, new RPC) must land a Pact CDC RED test before the producer GREEN. "Proto compiled + E2E green" is not enough — the silent-fallback failure mode (ADR-000928) hides DI/wiring gaps that defensive nil-guards swallow.
8. **No silent fallback for unwired dependencies** — Optional dependencies (DI options, feature flags, future hooks) must surface their wiring state with a loud `*_enabled` / `*_disabled` startup log AND `panic` when the unwired branch is taken inside business code. Defensive `if x == nil { return nil }` in producer / projector / resolver paths is forbidden — it makes "DI forgot to wire" indistinguishable from "intentionally disabled" (PM-2026-045 / ADR-000928 root cause). Details: `.claude/rules/di-wiring.md`.
9. **Fail-fast startup config** — Missing required config (secrets, upstream URLs, auth tokens) = exit non-zero at startup. Never warn-and-limp. "Disabled" must be an explicit config value with a startup log, never inferred from an unset variable.
10. **Stream consumers: ACK after durable write + reclaim loop** — XACK only after the side effect is durable (not on buffer-in). Every XREADGROUP consumer MUST run an XAUTOCLAIM reclaim loop, or crashed-consumer messages are lost forever and DLQ conditions never fire. Details: `.claude/rules/event-stream-consumer.md`.

## Planning

Obsidian vault (`docs/`) — 466+ ADRs, plans, runbooks, reviews. Run `/plan-context-loader`
before designing; it loads the canonical contracts and searches `docs/ADR/` directly.

## Immutable Data Model Invariants

- **Append-first**: State via events, not mutable flags. `knowledge_events` is INSERT-only.
- **Reproject-safe**: Projectors use event payload only, never latest state queries.
- **Versioned artifacts**: Summaries/tags use `summary_versions`/`tag_set_versions`.
- **Why as first-class**: Home items and Trail branches must explain why they were surfaced / proposed.
- **Disposable projections**: Read models (`knowledge_trail_footprints`, `knowledge_trail_branches`, `knowledge_home_items`, `today_digest_view`, `recall_candidate_view`) can be rebuilt from the event log.

Before planning, run `/plan-context-loader` to load the relevant ADRs first.
