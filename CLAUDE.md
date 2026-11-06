# Alt - AI-Augmented RSS Knowledge Platform

## WHAT

Monorepo with 20+ microservices (Go, Python, Rust, TypeScript, Deno). Docker Compose-first orchestration, TDD-first development.

Each service has its own `CLAUDE.md` with service-specific guidance. See `docs/services/MICROSERVICES.md` for the full reference.

**alt-backend / alt-harvester / alt-notifier / alt-data-hub は 1 ディレクトリ 4 バイナリ** —
`alt-backend/app` という単一 Go モジュール (`module alt`) から `cmd/backend`（ユーザ向け API）/
`cmd/harvester`（7 定期ジョブ）/ `cmd/notifier`（Web Push 配信、API listener なし）/
`cmd/datahub`（alt-db の唯一のオーナー、mTLS 専用）の 4 本を切り出し、同じ
`alt-backend/Dockerfile.backend` を `--build-arg BINARY=backend|harvester|notifier|datahub` で
4 イメージにビルドする。4 つとも `cd alt-backend/app && go test ./...` で一括してテストする。
`cmd/fix_article_titles` は `go build ./cmd/...` に含まれるが常駐コンテナではない一発実行スクリプト。
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
curl http://localhost/health             # Frontend (via plecto-proxy)
curl http://localhost:9000/v1/health    # Backend
curl http://localhost:9250/health       # BFF
curl http://localhost:7700/health       # Meilisearch
```

## Toolchain

### LKAL (`lkal`)
- **What**: Declarative configuration format (`@lkal "0.1"`) and official CLI parser/normalizer (`~/.local/bin/lkal`, installed via `make install-lkal`) backing `deploy/desired-env.lkal`.
- **Use when / Commands**:
  - Editing repository-managed host environment settings (`/upstreams`, `/mtls_allowlists`, `/rate_limits`, `/feature_flags`) in `deploy/desired-env.lkal` to stay in lockstep with `deploy/host-prereqs.yaml` and ADR-000982 (`RATE_LIMIT_EXTERNAL_API_INTERVAL >= 5s`); never declare `/local/*` in `deploy/desired-env.lkal`.
  - Check or format syntax with `~/.local/bin/lkal check deploy/desired-env.lkal` and `~/.local/bin/lkal fmt -w deploy/desired-env.lkal` (`scripts/kisage-env.py` normalizes via `lkal json -plain` with no regex fallback).
- **Verify**: `python3 scripts/kisage-env.py verify` (or `make kisage-verify`).

### kisage (`kisage` + `scripts/kisage-env.py`)
- **What**: Deterministic leaf-path LKAL reconciler and MCP server (`~/.local/bin/kisage` `v0.1.2`, installed via `make install-kisage`), paired with `scripts/kisage-env.py` and `docs/runbooks/kisage-env.md`.
- **Use when / Commands**:
  - Modifying `deploy/desired-env.lkal`, `deploy/host-prereqs.yaml`, or `scripts/kisage-env.py`: run `python3 scripts/kisage-env.py verify` and `python3 scripts/tests/test-kisage-env.py`.
  - Auditing an actual `.env` file against managed keys without printing secret values: run read-only `python3 scripts/kisage-env.py preflight --env-file .env` (`make kisage-preflight ENV_FILE=.env`).
  - Never edit `.env` or `.kisage/*` directly (blocked by `.claude/settings.json` `PreToolUse` hook) and never automate `kisage adopt` or `kisage prune`; stateful host reconciliation (`kisage plan|apply|prune|explain|adopt|release` or `kisage_*` MCP tools under `KISAGE_HOST_CONFIG="$PWD/.kisage/host.json"`) is host-operator scope per `docs/runbooks/kisage-env.md`.
- **Verify**: `python3 scripts/kisage-env.py verify && python3 scripts/tests/test-kisage-env.py` (plus `python3 scripts/kisage-env.py preflight --env-file .env` when inspecting a host `.env`).

### DocDag (`docdag`)
- **What**: Typed directed graph validator, rule linter, and query CLI (`docdag`, configured in `docdag.yaml`) enforcing DAG invariants and section structure across `docs/ADR/`.
- **Use when / Commands**:
  - Exploring ADR context, prerequisites, or blast radius before planning or reviewing (query the graph instead of grepping `docs/ADR/`): `docdag context <id> --depth 2 --budget 1500`, `docdag query --binding --fields id,title,status,path`, `docdag query <id>`, `docdag query <id> --ancestors`, `docdag resolve <id>`.
  - Creating an ADR: allocate the next 6-digit ID via `docdag new "<title>" --dry-run --format json`, write `docs/ADR/NNNNNN.md` with `docs/ADR/template.md` headings (`## Status` → `## Context` → `## Decision` → `## Consequences`), and check incrementally with `docdag validate --touching docs/ADR/NNNNNN.md`.
  - Modifying `docs/ADR/**` or `docdag.yaml`: settled ADRs are append-only (`--immutable-since origin/main`); only frontmatter `status: superseded` may be updated on a superseded ADR.
- **Verify**: `docdag validate && docdag lint --strict` (and `docdag validate --immutable-since origin/main` before finishing any `docs/ADR/` or `docdag.yaml` change).

## Antigravity MCP delegation

Use `antigravity_run` for a separate, bounded investigation or plan. Default to
`mode: "plan"` and `autonomy: "safe"`; request `accept-edits` only when the
task explicitly calls for changes. Continue a thread only with the exact
`conversation_id` returned by the prior call, and keep the same workspace.
The bridge permits one Antigravity call at a time. Do not ask a delegated turn
to recursively delegate work or launch this MCP server.

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

Obsidian vault (`docs/`) — ADRs (see `docs/ADR/`), plans, runbooks, reviews. Run `/plan-context-loader`
before designing; it loads the canonical contracts and searches `docs/ADR/` directly.

## Immutable Data Model Invariants

- **Append-first**: State via events, not mutable flags. `knowledge_events` is INSERT-only.
- **Reproject-safe**: Projectors use event payload only, never latest state queries.
- **Versioned artifacts**: Summaries/tags use `summary_versions`/`tag_set_versions`.
- **Why as first-class**: Home items and Trail branches must explain why they were surfaced / proposed.
- **Disposable projections**: Read models (`knowledge_trail_footprints`, `knowledge_trail_branches`, `knowledge_home_items`, `today_digest_view`, `recall_candidate_view`) can be rebuilt from the event log.

Before planning, run `/plan-context-loader` to load the relevant ADRs first.
