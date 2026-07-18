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
3. **Plan** – Announce a concise multi-step plan via `update_plan` and keep it current.
4. **Implement surgically** – Edit only what the task requires; use tight diff scopes with `apply_patch`.
5. **Verify** – Run the smallest meaningful test or build command for the affected area.
6. **Communicate** – Summarize changes, list tests, surface risks, and suggest next actions.

## Repository Map
- `alt-frontend-sv/` – SvelteKit 2 + Svelte 5 Runes client (TailwindCSS v4, Vitest, Playwright).
- `alt-backend/app/` – Go 1.26 HTTP API in Clean Architecture layers.
- `alt-backend/sidecar-proxy/` – Go egress proxy enforcing outbound policy.
- `pre-processor/app/` – Go feed and summarization worker with circuit breakers.
- `pre-processor-sidecar/app/` – Go scheduler for Inoreader ingestion (CronJob/deployment).
- `news-creator/app/` – FastAPI LLM service using Ollama via Clean Architecture.
- `tag-generator/app/` – FastAPI + Python 3.14 tag pipeline with ML components.
- `search-indexer/app/` – Go Meilisearch indexer and search API.
- `auth-hub/` – Go IAP service bridging Nginx and Ory Kratos.
- `auth-token-manager/` – Deno OAuth2 token refresher for Inoreader.
- `knowledge-sovereign/` – Go durable knowledge state owner (Trail / event log / projectors).
- `rask-log-forwarder/` & `rask-log-aggregator/` – Rust log pipeline (forwarder + ClickHouse aggregator).
- Support assets: `compose/compose.yaml` (include stacks), `Makefile`, `scripts/`, `docker/`, `db/`, `.github/`, root `tests/`.

## Core Tooling & Commands
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
  - Tests: `SERVICE_SECRET=test-secret pytest` (news-creator), `uv run pytest` (tag-generator)
  - Type/lint: `uv run mypy`, `uv run ruff check`, `uv run ruff format`
- **Rust services (rask-*)**
  - Unit/integration: `cargo test`
  - Benchmarks: `cargo bench` (when explicitly required)
- **Deno (auth-token-manager)**
  - Tests: `deno test`
- **Health checks**
  - Frontend (via nginx): `curl http://localhost/health`
  - Backend: `curl http://localhost:9000/v1/health`
  - BFF: `curl http://localhost:9250/health`
  - Meilisearch: `curl http://localhost:7700/health`
  - Auth Hub: `curl http://localhost:8888/health`

## Language Playbooks
- **Go 1.26** – Enforce Clean Architecture boundaries, use `log/slog`, wrap errors with context, propagate `context.Context`, throttle external calls (≥5 s between repeat host hits), prefer table-driven tests and GoMock fakes.
- **SvelteKit / TypeScript** – Strict TypeScript, Svelte 5 Runes only (`$state` / `$derived` / `$effect`), TailwindCSS v4, Vitest + Testing Library; use `bun run test` (not bare `bun test`).
- **Python (FastAPI)** – Dependency injection via containers, async handlers, pytest + `pytest-asyncio`, maintain golden datasets for LLM prompt regressions, sanitize LLM outputs.
- **Rust 2024** – Favor `async fn` in traits, zero-copy parsing, lock-free data structures, test with `axum-test`, benchmark critical code paths with `criterion`.
- **Deno TypeScript** – Use `@std/testing` BDD utilities, stub global fetch for token refresh tests, never log secrets.

## Service Capsules
- **alt-frontend-sv** – SvelteKit App Router at `/`, Alt-Paper theme, Runes-only. Tests via Vitest; Playwright E2E uses page objects. Lint/format (`biome`) before hand-off.
- **alt-backend** – Echo handlers → Usecase → Port → Gateway → Driver. Respect rate limiting (5 s external API gap). Use `log/slog` and structured error wrapping.
- **Sidecar Proxy** – Go reverse proxy enforcing outbound allowlists, shared timeouts, header normalization. Test with `net/http/httptest` triad (client → proxy → mock backend).
- **auth-hub** – Kratos session validator with 5-minute TTL cache. Exposes `/validate` and `/health`; ensure identity headers (`X-Alt-*`) are authoritative.
- **pre-processor** – Feed processing, summarization, quality gates. Uses circuit breakers (`mercari/go-circuitbreaker`), rate limits, structured logging per operation.
- **pre-processor-sidecar** – Scheduler for Inoreader OAuth2 ingestion. Runs as CronJob (Forbid concurrency). Uses `singleflight` for token refresh and pluggable clocks for testing.
- **news-creator** – FastAPI LLM orchestrator with Clean Architecture layers. Summaries produced via Ollama gateway. Tests mock ports, evaluate prompts via golden datasets and `DeepEval` where applicable.
- **tag-generator** – FastAPI ML service generating article tags. Emphasizes batch processing, memory hygiene, ML quality checks, and bias detection tests.
- **search-indexer** – Go service indexing to Meilisearch. Batch size 200, configures searchable/filterable attributes on startup. Integration tests require real Meilisearch.
- **knowledge-sovereign** – Durable knowledge event log + Trail/Home projectors. Own DB; consumers talk via RPC / events, not shared SQL.
- **auth-token-manager** – Deno service refreshing Inoreader tokens. Tests stub `fetch`, refactors only after Red/Green.
- **rask-log-forwarder** – Rust sidecar tailing Docker logs with SIMD parsing, lock-free buffers, disk fallback. Tests cover parsers, collectors, full pipeline with `wiremock`.
- **rask-log-aggregator** – Rust Axum API ingesting logs into ClickHouse. Uses mock traits for unit tests, `axum-test` for handlers, `criterion` for hot paths.

## Testing Matrix
- Frontend unit/component – `cd alt-frontend-sv && bun run test`
- Frontend lint/format – `cd alt-frontend-sv && bun run lint`, `bun run format`
- Frontend E2E – `cd alt-frontend-sv && bun run test:e2e` (requires stack)
- Backend Go suites – `cd alt-backend/app && go test ./...`
- Go side services – `go test ./...` in respective directories (add `-tags=integration` when noted)
- Python services – `pytest` (with required env), `uv run pytest`, `uv run mypy`, `uv run ruff check`
- Rust services – `cargo test` (optionally `cargo bench`/`criterion`)
- Deno service – `deno test`

## Security & Secrets
- Never commit real credentials. Base env files on `.env.template` and keep `.env` local.
- auth-hub, pre-processor, and news-creator rely on structured JSON logs—preserve redaction helpers.
- Sanitize LLM outputs; test against prompt-injection vectors (OWASP Top 10 for LLMs).
- Use provided SSL helpers (`make dev-ssl-setup`, `make dev-ssl-test`, `make dev-clean-ssl`) when working with local TLS.

## Delivery Checklist
- Plan updated; all steps completed or clearly marked.
- Changes minimal, relevant, and formatted with project tools (`bun run format`, `gofmt`, `ruff`, etc.).
- Appropriate tests executed and reported.
- No Compose/Kubernetes drift—legacy assets under `stopped-using-k8s/` untouched.
- Final message includes short rationale, file references, test evidence, and suggested next steps when applicable.

## Quick References
- Root context: `CLAUDE.md`
- Wiki entry: `docs/wiki/HOME.md` (Trail / sovereign map)
- Trail contracts: `docs/plan/knowledge-trail-core-concept.md`, `docs/plan/knowledge-trail-implementation-plan.md`
- Service deep dives: individual `CLAUDE.md` files in each service directory.
- Observability: include `compose/logging.yaml` (via `compose/compose.yaml`) to run `rask` services and inspect logs via ClickHouse.
- Health probes: `docker compose -f compose/compose.yaml -p alt ps`, `docker compose -f compose/compose.yaml -p alt logs -f <service>`; `kubectl` only when explicitly requested.
