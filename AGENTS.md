# AGENTS Playbook

## Mission & Scope
- Keep Alt's Compose-first stack healthy while accelerating developer workflows.
- Document the guardrails for AI coding agents and human collaborators working inside this monorepo.
- Treat Kubernetes assets in `stopped-using-k8s/` as historical only—do not modify them unless specifically asked.

## Operating Constraints (October 2025)
- Filesystem access: `workspace-write`; touch only the workspace and declared writable roots.
- Network access: restricted. Never run commands that require outbound traffic without approval.
- Approvals: `on-request`. Ask for escalation only when essential; otherwise work within the sandbox.
- Default tools: prefer `rg` for search and `sed -n` for focused reads; keep command output under 250 lines.
- Patching: use `apply_patch` for code and doc edits. Group related changes, avoid unrelated refactors.
- Planning: maintain a live plan with `update_plan`, exactly one step `in_progress` at a time.
- TDD First: every service expects Red → Green → Refactor before shipping implementation work.

## Baseline Workflow for Agents
1. **Confirm context** – Read this playbook plus relevant service `CLAUDE.md` files before touching code.
2. **Explore lightly** – Skim directories with `rg --files` and targeted `sed -n` reads. Avoid dumping large files.
3. **Plan & draft ADR** – Announce a concise multi-step plan via `update_plan`, check binding ADRs (`docdag context`), and draft `docs/ADR/NNNNNN.md` (`status: proposed`) before editing production code (tests and `docs/` are exempt).
4. **Implement surgically** – Edit only what the task requires; use tight diff scopes with `apply_patch`.
5. **Verify & finalize ADR** – Run the smallest meaningful test or build command for the affected area, promote the drafted ADR (`proposed` → `accepted`), and run `docdag validate`.
6. **Communicate** – Summarize changes, list tests, surface risks, and suggest next actions.

## Repository Map
- `alt-frontend-sv/` – SvelteKit 2 + Svelte 5 Runes client (TailwindCSS v4, Vitest, Playwright).
- `alt-backend/app/` – Go 1.27.1 HTTP API in Clean Architecture layers.
- `alt-backend/sidecar-proxy/` – Go egress proxy (historical; not run by compose).
- `pre-processor/app/` – Go feed and summarization worker with exponential-backoff retries.
- `pre-processor-sidecar/app/` – Go scheduler service for Inoreader ingestion.
- `news-creator/app/` – FastAPI LLM service using Ollama via Clean Architecture.
- `tag-generator/app/` – FastAPI + Python 3.14 tag pipeline with ML components.
- `search-indexer/app/` – Go Meilisearch indexer and search API.
- `auth-hub/` – Go session validator and backend token issuer for Ory Kratos.
- `auth-token-manager/` – Deno OAuth2 token refresher for Inoreader.
- `knowledge-sovereign/` – Go durable knowledge state owner (Trail / event log / projectors).
- `rask-log-forwarder/` & `rask-log-aggregator/` – Rust log pipeline (forwarder + ClickHouse aggregator).
- Support assets: `compose/compose.yaml` (include stacks), `Makefile`, `scripts/`, `docker/`, `db/`, `.github/`, root `tests/`.

## Core Tooling & Commands
- **Canonical lint gate (CI-identical)**
  - Module or named target: `./scripts/lint/run.sh <target>` (e.g. `./scripts/lint/run.sh alt-backend/app`, `./scripts/lint/run.sh repo`, or `make lint TARGET=<target>`).
  - Full repository gate: `./scripts/lint/run.sh all` (or `make lint`); list all valid targets with `./scripts/lint/run.sh --list` (or `make lint-list`).
- **Stack orchestration**
  - Prefer: `docker compose -f compose/compose.yaml -p alt up -d` (see root `CLAUDE.md`).
  - `make up` – Copies `.env.template` → `.env` if needed, builds images, starts Docker Compose.
  - `make down` / `make down-volumes` – Stop stack (keep vs. drop volumes).
  - Stacks are Compose **`include:`** files under `compose/` (`ai.yaml`, `logging.yaml`, `sovereign.yaml`, …), not root-level `--profile` flags.
- **Frontend (SvelteKit / bun)**
  - Dev server: `cd alt-frontend-sv && bun dev`
  - Build: `cd alt-frontend-sv && bun run build`
  - Tests: `cd alt-frontend-sv && bun run test` (unit), `bun run test:e2e` (requires stack), coverage via `test:coverage`
  - Quality gates: `cd alt-frontend-sv && bun run format`, `bun run lint`, `bun run check`
- **Backend & Go services**
  - Go tests: `cd <service> && go test ./...` (add `-race -cover` when appropriate)
  - Formatting: `gofmt`, linting via `go vet`
  - Mock generation: `make generate-mocks`
- **Python services (news-creator, tag-generator)**
  - Tests: `cd news-creator/app && uv run pytest`, `cd tag-generator/app && uv run pytest`
  - Type/lint: `uv run pyrefly check`, `uv run ruff check`, `uv run ruff format`
- **Rust services (rask-*)**
  - Unit/integration: `cargo test`
  - Benchmarks: `cargo bench` (when explicitly required)
- **Deno (auth-token-manager)**
  - Tests: `deno test`
- **Health checks**
  - Frontend (via plecto-proxy): `curl http://localhost/health`
  - Backend: `curl http://localhost:9000/v1/health`
  - BFF: `curl http://localhost:9250/health`
  - Meilisearch: `curl http://localhost:7700/health`
  - Auth Hub (in-container, port 8888 not published to host): `docker compose -f compose/compose.yaml -p alt exec auth-hub /auth-hub healthcheck`
- **Repo toolchain (`LKAL`, `kisage`, `DocDag`)**
  - Canonical rules, commands, and role boundaries live in root `CLAUDE.md` under `## Toolchain` (do not duplicate here).
  - Verify `deploy/desired-env.lkal` / `kisage` changes with `python3 scripts/kisage-env.py verify && python3 scripts/tests/test-kisage-env.py` (never edit `.env` or `.kisage/*` directly), and verify `docs/ADR/` or `docdag.yaml` changes with `docdag validate && docdag lint --strict`.

## Language Playbooks
- **Go 1.27.1** – Enforce Clean Architecture boundaries, use `log/slog`, wrap errors with context, propagate `context.Context`, throttle external calls (≥5 s between repeat host hits), prefer table-driven tests and GoMock fakes.
- **SvelteKit / TypeScript** – Strict TypeScript, Svelte 5 Runes only (`$state` / `$derived` / `$effect`), TailwindCSS v4, Vitest + Testing Library; use `bun run test` (not bare `bun test`).
- **Python (FastAPI)** – Dependency injection via containers, async handlers, pytest + `pytest-asyncio`, maintain golden datasets for LLM prompt regressions, sanitize LLM outputs.
- **Rust 2024** – Favor `async fn` in traits, zero-copy parsing, lock-free data structures, test with `axum-test`, benchmark critical code paths with `criterion`.
- **Deno TypeScript** – Use `@std/testing` BDD utilities, stub global fetch for token refresh tests, never log secrets.

## Service Capsules
- **alt-frontend-sv** – SvelteKit App Router at `/`, Alt-Paper theme, Runes-only. Tests via Vitest; Playwright E2E uses page objects. Lint/format (`biome`) before hand-off.
- **alt-backend** – Echo handlers → Usecase → Port → Gateway → Driver. Respect rate limiting (5 s external API gap). Use `log/slog` and structured error wrapping.
- **Sidecar Proxy** – Go reverse proxy (historical; compose does not run it). Outbound allowlists, timeouts, header normalization.
- **auth-hub** – Kratos session validator with 5-minute TTL cache. Exposes `/validate` and `/health`; ensure identity headers (`X-Alt-*`) are authoritative.
- **pre-processor** – Feed processing, summarization, quality gates. Uses exponential-backoff retries (RetryPolicy), rate limits, structured logging per operation.
- **pre-processor-sidecar** – Scheduler for Inoreader OAuth2 ingestion. Runs as a Compose daemon service. Uses `singleflight` for token refresh and pluggable clocks for testing.
- **news-creator** – FastAPI LLM orchestrator with Clean Architecture layers. Summaries produced via Ollama gateway. Tests mock ports, evaluate prompts via golden datasets and `DeepEval` where applicable.
- **tag-generator** – FastAPI ML service generating article tags. Emphasizes batch processing, memory hygiene, ML quality checks, and bias detection tests.
- **search-indexer** – Go service indexing to Meilisearch. Batch size 200, configures searchable/filterable attributes on startup. Integration tests require real Meilisearch.
- **knowledge-sovereign** – Durable knowledge event log + Trail/Home projectors. Own DB; consumers talk via RPC / events, not shared SQL.
- **auth-token-manager** – Deno service refreshing Inoreader tokens. Tests stub `fetch`, refactors only after Red/Green.
- **rask-log-forwarder** – Rust sidecar tailing Docker logs with SIMD parsing, lock-free buffers, disk fallback. Tests cover parsers, collectors, full pipeline with `wiremock`.
- **rask-log-aggregator** – Rust Axum API ingesting logs into ClickHouse. Uses mock traits for unit tests, `axum-test` for handlers, `criterion` for hot paths.

## Critical Rules
These rules and the invariants below are the same text as root `CLAUDE.md`; `./scripts/altharness/alt-harness selfcheck` fails when the two copies differ.

<!-- shared:critical-rules:start -->
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
11. **IMPORTANT: ADR First** — Before editing production code, check binding ADRs and draft `docs/ADR/NNNNNN.md` (`status: proposed`) via `docdag new "<title>" --dry-run --format json` + `docs/ADR/template.md`, then promote to `accepted` after tests pass (tests and `docs/` are exempt; enforced by `alt-harness preflight`, [[001032]]).
<!-- shared:critical-rules:end -->

## Immutable Data Model Invariants
<!-- shared:immutable-invariants:start -->
- **Append-first**: State via events, not mutable flags. `knowledge_events` is INSERT-only.
- **Reproject-safe**: Projectors use event payload only, never latest state queries.
- **Versioned artifacts**: Summaries/tags use `summary_versions`/`tag_set_versions`.
- **Why as first-class**: Home items and Trail branches must explain why they were surfaced / proposed.
- **Disposable projections**: Read models (`knowledge_trail_footprints`, `knowledge_trail_branches`, `knowledge_home_items`, `today_digest_view`, `recall_candidate_view`) can be rebuilt from the event log.
<!-- shared:immutable-invariants:end -->

## ADR Preflight Guardrail
- Workflow, hook behavior, and CLI flags live in `.agents/skills/adr-preflight/SKILL.md` (same files as `.claude/skills/adr-preflight/`). Production edits are blocked by `alt-harness preflight` across Claude Code, `agy-cli`, and Codex until `docdag validate` passes and a valid new/updated `docs/ADR/NNNNNN.md` exists (`docs/**`, `docdag.yaml`, and test files are exempt; no blind retry or `--skip`). Without edit hooks, run `./scripts/altharness/alt-harness preflight <paths>` before editing.
- Read listed binding ADRs (`docdag context <id> --depth 2 --budget 1500`) first; stop and get user approval before superseding one. Draft `status: proposed` before production edits (`alt-adr-writer --draft`) and promote to `accepted` after tests pass (`alt-adr-writer --finalize`).

## Clean Architecture Guardrail
- Import rules, the placement flow, and per-service exceptions live in `.agents/skills/clean-architecture/SKILL.md` (the same files as `.claude/skills/clean-architecture/`). Read it before adding an endpoint, usecase, port, gateway, or driver.
- After editing Go or Python layer code, run `bash .agents/skills/clean-architecture/scripts/check_layers.sh --changed` and fix every `VIOLATION`; each one comes with a `FIX` line. `KNOWN` lines are recorded debt, not part of your task, and must not grow. They are printed only by the whole-service form, `check_layers.sh <service-dir>`.

## Rule Ledger (CHECK-07)
- Every engineering rule in this playbook, `CLAUDE.md`, and `.agents/rules/` is paired with its concrete mechanical check (or marked `判断`) in [`.agents/rules/rule-ledger.md`](.agents/rules/rule-ledger.md).
- `./scripts/altharness/alt-harness selfcheck` (H11) validates all referenced scripts, `ast-grep:<id>` rules, corpus/ADR citations, and enforces a monotonic `max_judgment_rules` ratchet so prose rules migrate toward mechanical checks rather than accumulating unchecked.
- Worktree hygiene (`./scripts/altharness/alt-harness hygiene`) and disk-space / policy guards (`scripts/lint/run.sh`) are enforced mechanically; prefer updating the ledger over duplicating prose rules here.

## Testing Matrix
- Frontend unit/component – `cd alt-frontend-sv && bun run test`
- Frontend lint/format – `cd alt-frontend-sv && bun run lint`, `bun run format`
- Frontend E2E – `cd alt-frontend-sv && bun run test:e2e` (requires stack)
- Backend & Go services – `go test ./...` in the module directory (add `-tags=integration` when noted)
- Python services – `cd <service>/app && uv run pytest`, `uv run pyrefly check`, `uv run ruff check`
- Rust / Deno services – `cargo test` / `deno test`

## Security & Secrets
- Never commit credentials (`.env` / `secrets` / keys are guarded by `./scripts/altharness/alt-harness guard-protected`); preserve structured JSON log redaction in `auth-hub`, `pre-processor`, and `news-creator`, sanitize LLM outputs, and use `make dev-ssl-setup` for local TLS.

## Delivery Checklist
- Plan updated; changes minimal and verified via `./scripts/lint/run.sh <target>`, `./scripts/altharness/alt-harness hygiene`, and affected tests.
- `stopped-using-k8s/` untouched; final report includes rationale, file references, test evidence, and next steps.

## Quick References
- Root context: `CLAUDE.md`
- Wiki entry: `docs/wiki/HOME.md` (Trail / sovereign map)
- Trail contracts: `docs/plan/knowledge-trail-core-concept.md`, `docs/plan/knowledge-trail-implementation-plan.md`
- Service deep dives: individual `CLAUDE.md` files in each service directory.
- Observability: include `compose/logging.yaml` (via `compose/compose.yaml`) to run `rask` services and inspect logs via ClickHouse.
- Health probes: `docker compose -f compose/compose.yaml -p alt ps`, `docker compose -f compose/compose.yaml -p alt logs -f <service>`; `kubectl` only when explicitly requested.
