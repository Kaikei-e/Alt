# CLAUDE.md - The Alt Project

## Current Posture (October 2025)

Alt now runs primarily on Docker Compose using the root `compose.yaml`. Kubernetes and Skaffold manifests remain in the repo for future redeployments but are no longer part of day-to-day development. Unless a platform maintainer explicitly requests otherwise, treat Docker Compose (plus the provided Make targets) as the source of truth for local orchestration.

## Overview

Alt is an AI-augmented RSS knowledge platform composed of multiple services:

Before drilling into any microservice directory, open the matching `./docs/<service>.md` snapshot—those files stay closest to current contracts, dependencies, and test surfaces, so the service-level `CLAUDE.md` files can focus on process guardrails.

- **Frontend**: `alt-frontend` (Next.js 15 App Router, React 19, TypeScript 5.9)
- **Backend & APIs**: `alt-backend` (Go 1.24, Clean Architecture)
- **Processing & AI**: `pre-processor`, `tag-generator` (Python 3.13), `news-creator` (Ollama runtime)
- **Search**: `search-indexer` (Go) + Meilisearch 1.15.2
- **Observability**: `rask-log-forwarder` & `rask-log-aggregator` (Rust 1.87) + ClickHouse 25.6
- **Identity**: Kratos services (`kratos`, `kratos-db`, `auth-hub`)
- **Storage**: PostgreSQL 16, Meilisearch, ClickHouse

Every service adheres to TDD-first workflows, explicit contracts, and strict linting/formatting.

## Orchestration & Tooling

| Tooling | Purpose |
| --- | --- |
| `compose.yaml` | Defines the full multi-service stack, including optional profiles (`ollama`, `logging`) |
| `Makefile` | Idempotent wrappers (`make up`, `make down`, `make build`, SSL helpers, GoMock generation) |
| `.env.template` | Baseline environment variables; copied automatically when `make up` runs |
| `scripts/check-env.js` | Guards Next.js builds by ensuring required env vars are defined |

### Compose Profiles

- **Default**: Frontend, backend, Postgres, Meilisearch, search indexer, tag generator, Kratos stack, ClickHouse, log aggregator
- **`--profile ollama`**: Adds GPU-enabled `news-creator`, its volume init job, and the `pre-processor`
- **`--profile logging`**: Launches Rust log forwarders that stream container logs to the aggregator

Combine profiles via `docker compose --profile ollama --profile logging up --build` when you need both AI and observability pipelines.

### Daily Workflow

1. Ensure Docker Desktop (or compatible runtime) is running.
2. Seed configuration if needed: `cp .env.template .env` (or simply run `make up`).
3. Bring the core stack online: `make up` (builds images, starts containers, runs migrations).
4. Optionally enable profiles: `docker compose --profile ollama up -d`, etc.
5. Install project dependencies as needed (`pnpm -C alt-frontend install`, `go mod tidy`, `uv sync`).
6. Develop with TDD (see below); run targeted tests before pushing changes.
7. Shut down with `make down` (keep state) or `make down-volumes` (reset state).

Health checks and diagnostics:

- Frontend: `curl http://localhost:3000/api/health`
- Backend: `curl http://localhost:9000/v1/health`
- Meilisearch: `curl http://localhost:7700/health`
- Database readiness: `docker compose exec db pg_isready -U ${POSTGRES_USER}`
- Container insights: `docker compose logs -f <service>` and `docker compose ps --status=running`

## Development Principles

### Test-Driven Development

TDD is mandatory across services.

- **Go**: `go test ./...` (add `-race -cover` where appropriate); GoMock via `make generate-mocks`
- **TypeScript**: `pnpm -C alt-frontend test`, watch mode via `test:watch`, Playwright E2E via `test:e2e`
- **Python**: `uv run pytest` in service directories (e.g., `tag-generator/app`)
- **Rust**: `cargo test` inside log services; keep unsafe blocks out of production code

Typical loop: write failing test → minimal implementation → refactor → run targeted tests → run lint/format.

### Clean Architecture & Contracts

Most Go services follow `REST` → `Usecase` → `Port` → `Gateway` → `Driver`. Ports define stable interfaces; gateways handle external systems. Maintain strict boundaries when adding features. Update mocks alongside interface changes.

### Security & Observability

- Secrets live in `.env` (never commit real credentials); update `.env.template` with non-sensitive defaults only.
- Generate dev SSL material via `make dev-ssl-setup`; validate with `make dev-ssl-test`.
- Logging defaults to JSON; enable the `logging` profile when you need ClickHouse-backed analytics.
- Respect PII-handling guidelines—no tokens or email addresses in logs.

## Testing Matrix

| Area | Command |
| --- | --- |
| Frontend unit/component | `pnpm -C alt-frontend test` |
| Frontend lint/format | `pnpm -C alt-frontend lint` / `fmt` |
| Frontend E2E | `pnpm -C alt-frontend test:e2e` (stack must be running) |
| Backend Go tests | `cd alt-backend/app && go test ./...` |
| Search indexer | `cd search-indexer && go test ./...` |
| Tag generator | `cd tag-generator && uv run pytest` |
| Rust log services | `cd rask-log-aggregator && cargo test` (and similar for forwarder) |

Run only the suites relevant to your changes plus any dependent smoke tests.

## Service-Level Documentation

Always start with the snapshot under `./docs/<service>.md` before referencing a microservice directory; those files capture the latest contract, dependencies, and testing surface so you enter each service-specific `CLAUDE.md` with the freshest context.

Each service maintains its own `CLAUDE.md` (process/architecture guardrails) and a companion `docs/*.md` snapshot that describes the current implementation state:

- Backend API – `alt-backend/app/CLAUDE.md` + `docs/alt-backend.md`
- Frontend – `alt-frontend/CLAUDE.md` + `docs/alt-frontend.md`
- Auth Hub – `auth-hub/CLAUDE.md` + `docs/auth-hub.md`
- Auth Token Manager – `auth-token-manager/CLAUDE.md` + `docs/auth-token-manager.md`
- Pre-processor – `pre-processor/app/CLAUDE.md` + `docs/pre-processor.md`
- Pre-processor Sidecar – `pre-processor-sidecar/app/CLAUDE.md` + `docs/pre-processor-sidecar.md`
- News Creator – `news-creator/app/CLAUDE.md` + `docs/news-creator.md`
- Tag Generator – `tag-generator/app/CLAUDE.md` + `docs/tag-generator.md`
- Search Indexer – `search-indexer/app/CLAUDE.md` + `docs/search-indexer.md`
- Rask Log Forwarder – `rask-log-forwarder/app/CLAUDE.md` + `docs/rask-log-forwarder.md`
- Rask Log Aggregator – `rask-log-aggregator/app/CLAUDE.md` + `docs/rask-log-aggregator.md`

If you add a new service, scaffold its doc with architecture notes, env vars, test commands, and integration points.

## When to Touch Kubernetes/Skaffold

- Keep manifests in `stopped-using-k8s/` untouched unless explicitly asked.
- Production planning: coordinate with platform leads before editing `skaffold/` or Helm values.
- Any change to share: document the Compose equivalent first, then mirror to Kubernetes if required later.

## Quick Reference

- Start stack: `make up`
- Stop stack: `make down`
- Tear down state: `make down-volumes`
- Rebuild everything: `make build`
- Run backend tests: `cd alt-backend/app && go test ./...`
- Run frontend dev server: `pnpm -C alt-frontend dev`
- Enable AI pipeline: `docker compose --profile ollama up -d`
- Enable log forwarders: `docker compose --profile logging up -d`

Stay aligned with the Compose-first workflow, keep tests green, and avoid modifying deprecated Kubernetes assets unless requested.
