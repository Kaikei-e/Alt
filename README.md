[![Backend Go Tests](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml)
[![Frontend Unit Tests](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-unit-test.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-unit-test.yaml)
[![Tag Generator](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml)

# Alt – Compose-First AI Knowledge Platform

_Last reviewed on October 29, 2025._

> Compose-first knowledge platform that ingests RSS content, enriches it with AI, and serves curated insights with a unified developer workflow across Go, Python, Rust, Deno, and Next.js services.

## Table of Contents

- [Platform Snapshot](#platform-snapshot)
- [Architecture](#architecture)
- [Technology & Version Matrix](#technology--version-matrix)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [First-Time Setup](#first-time-setup)
  - [Compose Profiles](#compose-profiles)
  - [Developer Setup Checklist](#developer-setup-checklist)
- [Service Catalog](#service-catalog)
- [Service Deep Dives](#service-deep-dives)
- [Observability & Operations](#observability--operations)
- [Development Workflow & Testing](#development-workflow--testing)
- [Testing Playbook](#testing-playbook)
- [Data & Storage](#data--storage)
- [Security & Compliance](#security--compliance)
- [External Integrations](#external-integrations)
- [Contribution Checklist](#contribution-checklist)
- [Roadmap & Historical Context](#roadmap--historical-context)
- [Change Management & Communication](#change-management--communication)
- [Troubleshooting & FAQs](#troubleshooting--faqs)
- [Glossary](#glossary)
- [Reference Resources](#reference-resources)
- [Appendix](#appendix)

## Platform Snapshot

- Compose-first developer experience: `make up` builds images, runs Atlas migrations, and starts the full stack under Docker Compose v2 profiles.
- Clean Architecture across languages: Go services follow handler → usecase → port → gateway → driver, while Python, Rust, and Deno counterparts mirror the same contract-first approach.
- AI enrichment pipeline: pre-processor deduplicates and scores feeds, news-creator produces Ollama summaries, and tag-generator delivers ML-backed topical tags before articles surface in the UI.
- Search-ready delivery: search-indexer batches 200-document upserts into Meilisearch 1.15.2 with tuned searchable/filterable attributes and semantic-ready schema defaults.
- Observability built in: Rust rask log services stream structured JSON into ClickHouse 25.6, complemented by health endpoints and targeted dashboards.
- Identity at the edge: auth-hub validates Kratos sessions, emits authoritative `X-Alt-*` headers, and caches them for five minutes so downstream services remain auth-agnostic.
- TDD-first change management: every service mandates Red → Green → Refactor with exhaustive unit suites, integration hooks, and deterministic mocks before production merges.
- Developer ergonomics & safety: shared Make targets, lint/format tooling, env guards, and secrets hygiene keep onboarding fast and safe.
- Production parity: Compose profiles mirror production paths so GPU summarisation (`ollama`) and log pipelines (`logging`) toggle locally without ad-hoc scripts.

## Architecture

Alt is designed to keep local parity with production by centering on Docker Compose while preserving historical Kubernetes manifests for reference only.

### Compose Topology

```mermaid
flowchart LR
    classDef client fill:#e6f4ff,stroke:#1f5aa5,color:#0d1f33
    classDef edge fill:#f2e4ff,stroke:#8a4bd7,color:#34175f
    classDef core fill:#e8f5e9,stroke:#2f855a,color:#123524
    classDef optional fill:#fff4e5,stroke:#f97316,color:#772b07,stroke-dasharray:4 3
    classDef data fill:#fef3c7,stroke:#d97706,color:#5b3a06
    classDef observability fill:#fde4f7,stroke:#c026d3,color:#4a0d68

    Browser((Browser / Mobile Client)):::client
    Nginx["nginx reverse proxy<br/>ports 80 ➜ services"]:::edge
    UI["alt-frontend<br/>Next.js 15 + React 19"]:::core
    AuthHub["auth-hub<br/>Go IAP proxy"]:::edge
    Backend["alt-backend<br/>Go Clean Architecture"]:::core
    Sidecar["sidecar-proxy<br/>HTTP policy"]:::core
    PreProc["pre-processor<br/>Go ingestion"]:::core
    Scheduler["pre-processor-sidecar<br/>Cron scheduler"]:::optional
    News["news-creator<br/>FastAPI + Ollama"]:::optional
    Tags["tag-generator<br/>FastAPI ML"]:::core
    Indexer["search-indexer<br/>Go → Meilisearch"]:::core
    Postgres["PostgreSQL 16"]:::data
    Kratos["Ory Kratos 1.1"]:::data
    Meili["Meilisearch 1.15.2"]:::data
    ClickHouse["ClickHouse 25.6"]:::data
    RaskF["rask-log-forwarder<br/>Rust sidecar"]:::observability
    RaskA["rask-log-aggregator<br/>Rust Axum API"]:::observability
    External["External APIs / RSS / Inoreader"]:::optional

    Browser -->|HTTPS| Nginx
    Nginx -->|/| UI
    Nginx -->|/api| AuthHub
    AuthHub --> Backend
    UI -->|REST via AuthHub| Backend
    Backend --> Sidecar
    Backend --> Postgres
    Backend --> Meili
    Backend --> Kratos
    Sidecar --> External
    PreProc --> Backend
    PreProc --> Tags
    PreProc --> News
    Scheduler --> PreProc
    Tags --> Postgres
    News --> Postgres
    Indexer --> Postgres
    Indexer --> Meili
    RaskF --> RaskA --> ClickHouse
    RaskF -->|tails| Nginx
    RaskF -->|tails| Backend
    RaskF -->|tails| UI
```

### Data Intelligence Flow

```mermaid
flowchart LR
    classDef ingest fill:#e0f7fa,stroke:#00838f,color:#004d40
    classDef ai fill:#ffe0f0,stroke:#d81b60,color:#880e4f
    classDef storage fill:#fff4d5,stroke:#fb8c00,color:#5d2c00
    classDef surface fill:#e8f5e9,stroke:#388e3c,color:#1b5e20

    RSS[External RSS feeds]:::ingest --> Fetch[pre-processor<br/>Fetch & dedupe]:::ingest
    Fetch --> Score[Quality scoring + language detection]:::ingest
    Score --> RawDB[(PostgreSQL<br/>raw articles)]:::storage
    RawDB --> TagJob[tag-generator<br/>Batch tag extraction]:::ai
    RawDB --> SummaryJob[news-creator<br/>LLM summary templating]:::ai
    TagJob --> TagDB[(PostgreSQL<br/>article_tags)]:::storage
    SummaryJob --> SummaryDB[(PostgreSQL<br/>article_summaries)]:::storage
    TagDB --> IndexBatch[search-indexer<br/>200-doc upserts]:::ai
    SummaryDB --> IndexBatch
    IndexBatch --> Meili[(Meilisearch<br/>search index)]:::storage
    Meili --> API[alt-backend REST<br/>search + browse]:::surface
    API --> Frontend[alt-frontend UI<br/>Chakra themes]:::surface
```

### Identity & Edge Access

Nginx fronts every `/api` call with `auth_request`, sending it to auth-hub. auth-hub validates the session via Kratos `/sessions/whoami`, caches the result for five minutes, and forwards authoritative `X-Alt-*` headers. alt-backend trusts those headers for user context while delegating outbound HTTP to `sidecar-proxy`, which enforces HTTPS allowlists and shared timeouts.

#### Component Responsibilities

- **Client tier** – Next.js UI delivers responsive dashboards, handles optimistic interactions, and mirrors backend feature flags via `NEXT_PUBLIC_*` variables.
- **Edge tier** – Nginx terminates TLS (when enabled), normalises headers, triggers auth-hub checks, and fan-outs requests to backend APIs or static assets.
- **Core services** – alt-backend orchestrates domain logic, while pre-processor, tag-generator, news-creator, and search-indexer cooperate to enrich, store, and surface content.
- **Data tier** – PostgreSQL persists canonical entities, Meilisearch powers discovery, ClickHouse retains observability telemetry, and Kratos maintains identities.
- **Observability tier** – Rust rask services guarantee durable log delivery, enabling replay into ClickHouse dashboards and anomaly detectors.
- **Optional profiles** – `ollama` introduces GPU inference footprint, `logging` deploys extended telemetry, and additional bespoke profiles can be layered for experiments.

#### Deployment Interaction Diagram

```mermaid
sequenceDiagram
    participant User
    participant Browser
    participant Nginx
    participant AuthHub
    participant Kratos
    participant Backend
    participant Sidecar
    participant ExternalAPI

    User->>Browser: Request dashboard
    Browser->>Nginx: GET /api/articles
    Nginx->>AuthHub: auth_request /validate
    AuthHub->>Kratos: GET /sessions/whoami
    Kratos-->>AuthHub: Session payload
    AuthHub-->>Nginx: 200 + X-Alt-* headers
    Nginx->>Backend: GET /api/articles (with headers)
    Backend->>Sidecar: Fetch RSS feed (if stale)
    Sidecar->>ExternalAPI: GET https://example.com/rss
    ExternalAPI-->>Sidecar: RSS XML
    Sidecar-->>Backend: Normalised response
    Backend-->>Browser: Article JSON payload
    Browser-->>User: Rendered dashboard
```

## Technology & Version Matrix

| Layer | Primary Tech | Version (Oct 2025) | Notes |
| --- | --- | --- | --- |
| Web UI | Next.js 15, React 19, TypeScript 5.9, pnpm 10.20.0 | Node.js 24 LTS | Chakra UI theme trio; App Router; Playwright + Vitest. |
| Go API & Proxy | Go 1.25, Echo, `net/http/httputil` | Go 1.25.x | Clean Architecture with GoMock; `testing/synctest`; sidecar enforces HTTPS allowlists and shared timeouts. |
| Go Data Pipeline | Go 1.25, `mercari/go-circuitbreaker`, `singleflight` | - | Pre-processor, scheduler, search-indexer; rate limit ≥5 s; 200-doc Meilisearch batches. |
| Python AI Services | Python 3.11/3.14, FastAPI, Ollama, `uv` | Ollama 0.3.x | Clean Architecture; golden prompts; bias detection; Ruff/mypy gates. |
| Identity & Tokens | Ory Kratos 1.1, auth-hub (Go 1.25), Deno 2.5.4 | - | 5-minute TTL cache; emits `X-Alt-*` headers; Inoreader refresh via `@std/testing/bdd`. |
| Observability | Rust 1.90 (2024 edition), ClickHouse 25.6 | - | SIMD log forwarder; Axum aggregator; `criterion` benchmarks. |
| Storage & Search | PostgreSQL 16, Meilisearch 1.15.2 | - | Atlas migrations; tuned searchable/filterable attributes; persisted volumes. |
| Orchestration | Docker Desktop 4.36+, Compose v2.27+, Makefile | - | `make up/down/build`; optional `ollama` and `logging` profiles; `.env.template` as source. |

> **Version cadence:** Go/Rust toolchains track stable releases quarterly, Next.js updates follow LTS adoption, and Python runtimes are pinned per service to avoid cross-environment drift. Update the matrix whenever upgrade stories land.

## Getting Started

### Prerequisites

- Docker Desktop 4.36+ (or Colima/Lima with Compose v2.27+) with at least 4 CPU / 8 GB memory allocated.
- Node.js 24 LTS with `pnpm` ≥9 installed globally (`corepack enable pnpm`).
- Go 1.25.x toolchain with `GOBIN` on your `PATH`.
- Python 3.14 (for tag-generator) and Python 3.11 (for news-creator) with `uv` for environment management.
- Rust 1.90 (2024 edition) and Cargo, including `rustup target add wasm32-unknown-unknown` if you run front-end bridges.
- Deno 2.5.4 and optional GPU runtime (CUDA 12+) if you plan to run Ollama locally.

### First-Time Setup

1. **Install dependencies** – run `pnpm -C alt-frontend install`, `uv sync --project tag-generator/app`, `uv sync --project news-creator/app`, and `go mod download ./...`.
2. **Seed environment** – copy `.env.template` to `.env`; `make up` performs this automatically if the file is missing.
3. **Start the stack** – execute `make up` to build images, run Atlas migrations, seed Meilisearch, and boot the default profile.
4. **Verify health** – hit `http://localhost:3000/api/health`, `http://localhost:9000/v1/health`, `http://localhost:7700/health`, and `http://localhost:8888/health`.
5. **Stop or reset** – use `make down` to stop while retaining volumes or `make down-volumes` to reset data.

### Compose Profiles

- **Default** – Frontend, backend, PostgreSQL, Kratos, Meilisearch, search-indexer, tag-generator, ClickHouse, rask-log-aggregator.
- **`--profile ollama`** – Adds news-creator (FastAPI + Ollama) and pre-processor ingestion services with persistent model volume at `news_creator_models`.
- **`--profile logging`** – Launches rask-log-forwarder sidecars that stream container logs into the aggregator; includes `x-rask-env` defaults.

Enable combinations as needed with `docker compose --profile ollama --profile logging up -d`.

### Developer Setup Checklist

1. **Install toolchains** – Docker Desktop/Colima, Go 1.25.x, Node.js 24 + `pnpm`, Python 3.11/3.14 with `uv`, Rust 1.90, and Deno 2.5.4 should all respond to `--version`.
2. **Bootstrap dependencies** – Run `pnpm -C alt-frontend install`, `uv sync` for Python services, `go mod download ./...`, and `cargo fetch` to warm caches.
3. **Prepare environment** – Copy `.env.template` to `.env`, fill local-safe secrets, and confirm `scripts/check-env.js` passes.
4. **Smoke the stack** – Execute `pnpm -C alt-frontend build`, `go test ./...`, `uv run pytest`, `cargo test`, then `make up`/`make down` to validate orchestration.
5. **Align practices** – Read root/service `CLAUDE.md`, enable editor format-on-save, install optional pre-commit hooks, and keep credentials out of git.

## Service Catalog

The list below summarises each microservice's responsibilities. Consult the directory-specific `CLAUDE.md` before implementing changes.

- **`alt-frontend/`** – Next.js 15 + React 19 + TS 5.9; Chakra UI themes; Vitest & Playwright POM; env guard script.
- **`alt-backend/app/`** – Go 1.25 Clean Architecture (handler → usecase → port → gateway → driver); GoMock & `testing/synctest`; outbound via sidecar; Atlas migrations.
- **`alt-backend/sidecar-proxy/`** – Go 1.25 proxy; HTTPS allowlists; shared timeouts; structured slog; `httptest` triad.
- **`pre-processor/app/`** – Go 1.25 ingestion; `mercari/go-circuitbreaker`; ≥5 s host pacing; structured logging.
- **`pre-processor-sidecar/app/`** – Go 1.25 scheduler; `singleflight` token refresh; injectable `Clock`; Cron/admin endpoints.
- **`news-creator/app/`** – Python 3.11 FastAPI; 5-layer Clean Architecture; Ollama Gemma 3 4B; golden prompts; `pytest-asyncio`.
- **`tag-generator/app/`** – Python 3.14 FastAPI ML pipeline; bias/robustness tests; manual GC; Ruff/mypy gates.
- **`search-indexer/app/`** – Go 1.25 + Meilisearch 1.15.2; 200-doc batches; checkpointed upserts; index settings on boot.
- **`auth-hub/`** – Go 1.25 IAP; 5-minute TTL cache; emits `X-Alt-*`; table-driven tests.
- **`auth-token-manager/`** – Deno 2.5.4 service; Inoreader OAuth2 refresh; `@std/testing/bdd`; sanitized logging.
- **`rask-log-forwarder/app/`** – Rust 1.90; SIMD JSON parsing; lock-free buffers; disk fallback resilience.
- **`rask-log-aggregator/app/`** – Rust 1.90 Axum; zero-copy ingestion; `axum-test`; `criterion` benchmarks; ClickHouse sink.

- Every service enforces Red → Green → Refactor and propagates structured logs or request IDs.
- Use the appendix command cheat sheet for the most common workflows.

## Service Deep Dives

- **alt-frontend** – Next.js 15 App Router UI with Chakra theming, SWR/react-query caching, and middleware-protected routes; Vitest + Playwright guard quality.
- **alt-backend/app** – Go 1.25 Clean Architecture API (handler → usecase → port → gateway → driver) with GoMock tests, Atlas migrations, and slog logging.
- **alt-backend/sidecar-proxy** – Go HTTP proxy centralising allowlists, timeouts, and header normalisation; exercised with three-part `httptest`.
- **pre-processor/app** – Go ingestion service fetching RSS feeds, deduping entries, and firing summarisation/tagging with circuit breakers and 5-second pacing.
- **pre-processor-sidecar/app** – Go scheduler managing Inoreader OAuth refresh with `singleflight`, deterministic clocks, and Cron-friendly endpoints.
- **news-creator/app** – FastAPI LLM orchestrator (Gemma 3 4B via Ollama) with Clean Architecture, golden prompts, and async pytest.
- **tag-generator/app** – Python 3.14 ML pipeline batching tag extraction, bias checks, and Postgres upserts under Ruff/mypy gates.
- **search-indexer/app** – Go service merging article, tag, and summary data into Meilisearch in 200-doc batches with checkpoints and index settings.
- **auth-hub** – Go Identity-Aware Proxy validating Kratos sessions, caching five-minute identities, and emitting canonical `X-Alt-*` headers.
- **auth-token-manager** – Deno 2.5.4 OAuth refresh worker stubbing `fetch` via `@std/testing/bdd` to keep rotations safe.
- **rask-log-forwarder/app** – Rust sidecar tailing Docker logs with SIMD parsing, lock-free queues, and disk fallback before ClickHouse handoff.
- **rask-log-aggregator/app** – Rust Axum ingestion API performing zero-copy parsing and persisting structured logs into ClickHouse with `criterion`.

- **Cross-cutting note** – Structured logging, context propagation, deterministic tests, and environment-driven configuration are mandatory across all services. Consult each service’s `CLAUDE.md` for precise commands, env vars, and gotchas before committing changes.

## Observability & Operations

- Enable the `logging` profile to run rask-log-forwarder sidecars; defaults stream 1 000-log batches (flush 500 ms) to `http://rask-log-aggregator:9600/v1/aggregate`. ClickHouse data lives in `clickhouse_data` and is accessible via `docker compose exec clickhouse clickhouse-client`.
- Monitor core endpoints below; Kratos (`http://localhost:4433/health/ready`) and ClickHouse (`http://localhost:8123/ping`) should also respond during smoke tests.

  | Service | Endpoint | Expectation |
  | --- | --- | --- |
  | Frontend | `http://localhost:3000/api/health` | `{ "status": "ok" }` |
  | Backend | `http://localhost:9000/v1/health` | `{ "status": "ok" }` |
  | Auth Hub | `http://localhost:8888/health` | HTTP 200 |
  | Meilisearch | `http://localhost:7700/health` | `{ "status": "available" }` |

- Use `docker compose logs -f <service>` for quick debugging, query ClickHouse for high-volume analysis, and run `backup-postgres.sh` / `backup-postgres-docker.sh` only when the stack is quiesced.

## Development Workflow & Testing

- Follow Red → Green → Refactor, starting with business-layer tests and regenerating mocks when interfaces evolve.
- Keep changes surgical and deterministic—lean on dependency injection, fake clocks (`testing/synctest`, custom `Clock`), and table/parameterized tests.
- Run formatters before committing (`pnpm fmt`, `gofmt`, `uv run ruff format`, `cargo fmt`, `deno fmt`) and document any new env vars or migrations.

### Test Matrix

| Area | Scope | Command | Notes |
| --- | --- | --- | --- |
| Frontend unit | alt-frontend components | `pnpm -C alt-frontend test` | Vitest + Testing Library + `userEvent`. |
| Frontend e2e | alt-frontend Playwright POM | `pnpm -C alt-frontend test:e2e` | Requires `make up`. |
| Go services | alt-backend/app, sidecar-proxy, pre-processor/app, pre-processor-sidecar/app, search-indexer/app, auth-hub | `go test ./...` from each directory | Add `-race -cover` when touching concurrency; regenerate mocks via `make generate-mocks`. |
| Python services | news-creator/app, tag-generator/app | `SERVICE_SECRET=test-secret pytest`; `uv run pytest` | Async tests use `pytest-asyncio`; Ruff and mypy enforce quality gates. |
| Rust services | rask-log-forwarder/app, rask-log-aggregator/app | `cargo test` | Performance guardrails via `cargo bench`. |
| Deno service | auth-token-manager | `deno test` | BDD-style assertions with `@std/testing/bdd`. |
| Compose smoke | Full stack health | `make up` then `curl` health endpoints | Confirms migrations, Meilisearch settings, and auth-hub session flow. |

## Testing Playbook

Alt’s quality bar depends on disciplined, layered tests:

- **Unit** – Pure functions, usecases, and adapters using table-driven Go tests, pytest fixtures, Vitest `describe.each`, or Rust unit modules.
- **Integration** – Boundary checks (Go ↔ Postgres, FastAPI ↔ Ollama mock, Rust ↔ ClickHouse) run via Compose services or lightweight doubles.
- **End-to-end** – Playwright journeys ensure auth headers, summarisation flows, and search UX remain intact; rely on Page Object Models.
- **Golden/Performance** – Guard LLM prompts and hot paths with golden datasets and `criterion`/`testing.B` benchmarks.

Authoring guidelines: name tests descriptively, isolate dependencies (GoMock, `pytest-mock`, `mockall`, `@std/testing/mock`), control time via fake clocks, and keep suites fast to avoid flaky CI.

CI expectations: PRs run lint + unit suites per language plus targeted integration/E2E jobs when code touches those areas. Record commands and outcomes in PR descriptions. If tests fail, prefer fixing root causes over blanket retries; update snapshots only when behaviour changes intentionally.

## Data & Storage

- PostgreSQL 16 (`db_data`) stores canonical entities: `feeds`, `articles`, `article_summaries`, `article_tags`, `ingestion_jobs`, `users`, and audit tables. Atlas migrations live in `migrations-atlas/` and must remain backward-compatible because `make up` replays them on every boot.
- Kratos maintains its own database (`kratos_db_data`) for identity state; never cross-link application tables to Kratos schemas—consume identity via auth-hub headers instead.
- Meilisearch (`meili_data`) holds denormalised search documents built by `search-indexer`; run `docker compose exec meilisearch index list` to inspect configured indices.
- ClickHouse (`clickhouse_data`) captures structured logs from rask-aggregator, enabling time-series queries, dashboards, and anomaly alerts.
- Backups: `backup-postgres.sh` (local Docker) and `backup-postgres-docker.sh` (Compose-aware) provide snapshot scripts; schedule them before major migrations. ClickHouse backups can be scripted via `clickhouse-client` or S3-based storage (future).

### Data Model Overview

```mermaid
erDiagram
    FEEDS ||--o{ ARTICLES : contains
    ARTICLES ||--o{ ARTICLE_SUMMARIES : summarised_by
    ARTICLES ||--o{ ARTICLE_TAGS : tagged_with
    ARTICLES }o--o{ INGESTION_JOBS : processed_in
    USERS ||--o{ ARTICLES : archived_by
```

### Storage Guardrails

- **Retention** – Articles stay until explicitly archived; summaries and tags follow cascading rules defined in migrations—avoid manual deletes.
- **Indices** – Postgres indexes `(feed_id, archived)` and `(published_at DESC)` keep queries snappy; adjust Meilisearch filterable attributes when adding new facets.
- **Migrations** – Preview drift with Atlas when available; keep changes idempotent and reversible.
- **Resets** – `make down-volumes` clears state; note any seed scripts so teammates can repopulate fixtures quickly.

## Security & Compliance

- Never commit real credentials; keep developer defaults in `.env.template` and load real secrets via `.env` or Kubernetes Secrets.
- auth-hub is the single source of identity—consume `X-Alt-*` headers and reject conflicting user context.
- Sanitize logs and use the TLS helpers (`make dev-ssl-setup`, `make dev-ssl-test`, `make dev-clean-ssl`) to keep traffic encrypted while redacting sensitive fields.
- Validate inputs, prefer parameterized queries, and wrap errors with context without leaking private details.

## External Integrations

- **Inoreader OAuth2** – Managed by `auth-token-manager` and `pre-processor-sidecar`; tokens refresh proactively and live in Secrets.
- **Ollama (Gemma 3 4B)** – Powers LLM summaries; install GPU drivers before enabling the `ollama` profile.
- **RSS & downstream connectors** – pre-processor respects publisher rate limits today; planned notification channels must preserve the same guardrails.

## Contribution Checklist

- Read the root and service-specific `CLAUDE.md` files before making changes.
- Start every change with a failing test and keep the affected suites green.
- Run formatters/linters and document new configuration, migrations, or APIs.
- Prove the change with the smallest meaningful test or health probe and note the result.
- Leave `stopped-using-k8s/` untouched unless asked and verify `make up` succeeds after edits.

## Roadmap & Historical Context

- Upcoming initiatives: extend auth-hub with tenant scoping, add semantic embeddings to Meilisearch, deliver live article status (SSE/WebSocket), and harden ClickHouse dashboards.
- Historical posture: Kubernetes assets in `stopped-using-k8s/` and the legacy `Alt-arch.mmd` diagram are reference-only—Compose remains the authoritative workflow.

## Change Management & Communication

- **Planning** – Open a GitHub issue or ADR for significant architectural work. Reference impacted services and note required Compose profile changes or migrations.
- **Documentation** – Update this README, relevant `CLAUDE.md`, and any runbooks when behaviour changes. Diagram diffs (Mermaid, Alt-arch.mmd) should be kept in sync.
- **Code reviews** – Default to reviewer pairs who own affected services; call out risky areas, test coverage, and rollback strategy. Highlight any rate limit, security, or compliance implications.
- **Release cadence** – Weekly Compose releases roll forward once smoke tests pass. Emergency fixes require tagged releases with changelog entries and communication in #alt-platform.
- **Communication channels** – Use #alt-dev for day-to-day collaboration, #alt-ops for incident coordination, and the platform RFC Notion space for long-form proposals.
- **Post-merge validation** – After merging, run `make up`, verify health endpoints, and monitor ClickHouse dashboards for anomalies during the first ingestion cycle.

## Troubleshooting & FAQs

| Symptom | Likely Cause | Resolution |
| --- | --- | --- |
| `pnpm dev` fails with missing env vars | `.env` not aligned with `.env.template` | Re-run `cp .env.template .env`, ensure `scripts/check-env.js` passes. |
| Backend returns 401 despite valid session | auth-hub cache stale or Kratos offline | Restart auth-hub container; verify Kratos `/sessions/whoami` responds; purge cache by restarting service. |
| Meilisearch searches empty after ingest | search-indexer not running or index misconfigured | Check `docker compose logs search-indexer`; rerun `search-indexer` manually; confirm index settings via Meili dashboard. |
| Ollama summary timeouts | Model not pulled or GPU unavailable | Run `docker compose --profile ollama logs news-creator`; preload model with `ollama pull gemma:4b`; confirm GPU drivers. |
| Rust services crash on startup | Insufficient ulimit or missing env | Ensure `LOG_LEVEL` and `RASK_ENDPOINT` set; increase file descriptors via Docker Compose `ulimits`. |
| Go tests flaky with timeouts | Missing fake clock or context deadline | Inject `testing/synctest` clock, set explicit deadlines, and avoid sleeping blindly in tests. |
| Playwright tests hang | Stack not running or selectors outdated | Start stack with `make up`; update POM selectors to match `data-testid` or page changes. |

**General tip:** Use `docker compose ps` and `docker compose logs -f <service>` for health checks, `docker compose exec db psql -U $POSTGRES_USER $POSTGRES_DB` for database inspection, and `make down-volumes` to reset state (only when data loss is acceptable).

## Glossary

- **Alt** – The Compose-first AI knowledge platform described in this repository.
- **Clean Architecture** – Layered approach separating interface (handlers), business logic (usecases), and infrastructure (gateways/drivers).
- **Compose profile** – Named group of services (e.g., `ollama`, `logging`) that can be toggled on via `docker compose --profile`.
- **Golden dataset** – Curated set of inputs/outputs used to detect regressions in LLM or ML-driven features.
- **IAP (Identity-Aware Proxy)** – Pattern where an edge service (auth-hub) centralises authentication before requests reach backend services.
- **LLM** – Large Language Model; in this project, Ollama-powered Gemma 3 4B produces article summaries.
- **Meilisearch** – Lightweight search engine used for full-text indexing and filtering of enriched content.
- **Rask** – Codename for the Rust observability duo: `rask-log-forwarder` (ingest) and `rask-log-aggregator` (ClickHouse sink).
- **Singleflight** – Go concurrency primitive ensuring only one duplicate request executes; used for token refresh.
- **TDD** – Test-Driven Development; the Red → Green → Refactor cycle enforced across all services.

## Reference Resources

- Internal docs: `CLAUDE.md` (root) and service-specific `CLAUDE.md` files.
- Architecture diagrams: `README.md` Mermaid blocks and `Alt-arch.mmd` for historical Kubernetes topology.
- Runbooks: `docs/` folder (if present) and scripts under `scripts/` for auth checks, log collection, and index resets.
- External references: [Next.js](https://nextjs.org/docs), [Go 1.25](https://go.dev/doc/devel/release), [Meilisearch](https://www.meilisearch.com/docs), [ClickHouse](https://clickhouse.com/docs), [Ollama](https://ollama.com/), [Kratos](https://www.ory.sh/kratos/docs/).
- Communication hubs: #alt-dev, #alt-ops Slack channels, and the Alt Notion workspace for RFCs and runbooks.

## Appendix

### Command Cheat Sheet

```bash
make up
make down
docker compose --profile ollama --profile logging up -d
pnpm -C alt-frontend test
cd alt-backend/app && go test ./...
```

### Essential Environment Variables

| Variable | Purpose | Default/Location |
| --- | --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Alt backend database credentials | `.env.template` |
| `KRATOS_INTERNAL_URL`, `KRATOS_PUBLIC_URL` | Ory Kratos internal/public endpoints | `.env.template` |
| `AUTH_HUB_INTERNAL_URL` | Internal URL for auth-hub | `.env.template` |
| `NEXT_PUBLIC_APP_ORIGIN`, `NEXT_PUBLIC_RETURN_TO_DEFAULT` | Frontend routing and redirects | `.env.template` |
| `SIDECAR_PROXY_BASE_URL`, `RASK_ENDPOINT` (+ batching vars) | Outbound proxy and log pipeline config | `.env` or compose `x-rask-env` |
| `SERVICE_SECRET`, `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET` | News-creator tests and Inoreader OAuth tokens | Export locally or inject via Secrets |

Keep `.env.template` updated with non-sensitive defaults whenever configuration changes, and mirror new variables here.

## Open-Source Credits

Alt stands on the shoulders of many open-source projects. We gratefully acknowledge the communities that maintain the tools and frameworks powering this platform.

- **Docker & Docker Compose** – Container orchestration backbone for local and CI workflows. © Docker Inc. Licensed under Apache 2.0/MIT.
- **Node.js & pnpm** – JavaScript runtime and package manager enabling the Next.js frontend. Node.js is available under the MIT License; pnpm under MIT.
- **Next.js & React** – Frontend framework and UI library (MIT) by Vercel and Meta. Chakra UI (MIT) provides the design system.
- **Go** – Systems language (BSD-style license) powering backend, ingestion, proxy, and auth services. Includes Echo (MIT), GoMock (Apache 2.0), and other Go ecosystem libraries.
- **Python & FastAPI** – Python (PSF License) with FastAPI (MIT) drives LLM summarisation and tagging services, alongside `uv` (MIT), `pytest` (MIT), and the wider scientific stack (varied permissive licenses).
- **Rust** – Rust compiler/toolchain (Apache 2.0 / MIT dual license) underpins observability sidecars, supplemented by crates like Axum (MIT/Apache 2.0), Tokio (MIT/Apache 2.0), and Criterion (Apache 2.0).
- **Deno** – Secure TypeScript/JavaScript runtime (MIT) powering the auth-token-manager.
- **Ollama** – Open-source LLM runtime (MIT) providing Gemma 3 4B hosting for news-creator.
- **Meilisearch** – Search engine (MIT) delivering full-text indexing. Integrated via official Go client (MIT).
- **PostgreSQL & pgx** – PostgreSQL database (PostgreSQL License) and associated drivers for data persistence.
- **ClickHouse** – Columnar database (Apache 2.0) backing observability analytics.
- **Ory Kratos** – Identity infrastructure (Apache 2.0) enabling session validation via auth-hub.
- **Mercari/go-circuitbreaker, singleflight, slog, Atlas** – Key Go libraries (varied BSD/MIT/Apache licenses) supporting resilience, logging, and migrations.
- **Testing frameworks** – Vitest, Playwright, pytest, Go `testing`, Cargo test, and Deno test suites (MIT/BSD/Apache) enforcing the TDD workflow.
- **Linux base images** – Alpine Linux (MIT), Debian/Ubuntu (varied open-source licenses) form the runtime foundation for service containers.

Each dependency retains its respective license; review individual repositories for full terms. We remain committed to upstream contributions and timely upgrades to honour these communities.
