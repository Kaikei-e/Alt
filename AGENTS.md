# AGENTS Guide

This document is the operational handbook for AI coding agents and humans collaborating via Codex CLI in this repository. It explains how to work safely, predictably, and fast within the project’s constraints.

## Audience & Purpose
- Agents: Use this as your canonical workflow and rules of engagement.
- Humans: Use this to understand how agents operate and what to expect.

## Quick Start (Agents)
- Confirm constraints: Filesystem `workspace-write`, network `restricted`, approvals `on-request` unless stated otherwise.
- Explore first: Prefer `rg` and targeted `sed -n` reads. Avoid large dumps (>250 lines).
- Create a plan: Use `update_plan` with short steps, one `in_progress` at a time.
- Patch surgically: Use `apply_patch` with minimal, scoped changes; don’t re-read files you just wrote unless needed.
- Test what you touch: Run narrow tests/builds related to the change; avoid unrelated fixes.
- Format/lint locally: Use repo-provided scripts; do not introduce new formatters.
- Never commit unless asked: Leave commits/branches to maintainers unless explicitly requested.
- Summarize clearly: Final messages should be concise, actionable, and list next steps if any.

## Monorepo Layout
- `alt-frontend/`: Next.js app (TypeScript) with Vitest and Playwright. Source under `src/`; tests in `tests/` and co‑located `*.test.ts(x)`.
- `alt-backend/app/`: Go 1.24 service (Clean Architecture). Domains under `domain/`, use cases under `usecase/`, adapters in `gateway/` and `driver/`. HTTP in `rest/`. Tests beside code; mocks in `mocks/`.
- `compose.yaml` and `Makefile`: Local orchestration and common tasks.
- Other services: `pre-processor/`, `search-indexer/`, `tag-generator/`, `auth-service/`.
- Infra & assets: `docker/`, `scripts/`, `db/`, `nginx/`, `.github/`, root `tests/`.

## Core Commands
- Stack up: `make up` (creates `.env` from `.env.template` if missing; builds and starts Docker Compose)
- Stack down/clean: `make down` | `make down-volumes` | `make clean`
- Frontend dev: `pnpm -C alt-frontend dev` (Next.js dev server)
- Frontend build: `pnpm -C alt-frontend build`
- Frontend tests: `pnpm -C alt-frontend test` | coverage `pnpm -C alt-frontend test:coverage` | E2E `pnpm -C alt-frontend test:e2e`
- Backend tests: `cd alt-backend/app && go test ./...` (add `-race -cover` as needed)
- Generate mocks: `make generate-mocks`
- Dev DB SSL: setup `make dev-ssl-setup` | verify `make dev-ssl-test` | clean `make dev-clean-ssl`

## Codex CLI Operating Rules
- Preambles: Before tool calls, briefly state what you’re about to do (1–2 sentences).
- Planning: Maintain a live plan via `update_plan`; exactly one step `in_progress` until complete.
- Shell usage: Prefer `rg` for search and `sed -n` to read specific ranges; keep outputs under 250 lines.
- Escalations: If a command needs network or broader filesystem access, request with justification; only when necessary.
- Patching: Use `apply_patch` exclusively for file changes. Group related edits; avoid unrelated refactors.
- Testing: Start specific (the changed package) then broaden only if useful. Do not fix unrelated failing tests.
- Formatting: Use existing project scripts (`pnpm -C alt-frontend fmt`, `lint`; Go uses `gofmt`/`go vet`).
- No commits by default: Do not run `git commit` or create branches unless explicitly asked.

## Style & Conventions
- TypeScript/React
  - Indent 2 spaces. Components PascalCase; hooks named `useX`; tests `*.test.ts(x)`.
  - Run `pnpm -C alt-frontend fmt` and `pnpm -C alt-frontend lint` before handing off.
- Go
  - Use `gofmt` defaults; `go vet` clean. Package names lower-case; exported types/functions PascalCase; file names snake_case where idiomatic.
- Configuration
  - Base envs on `.env.template`; never commit secrets or local logs.

## Testing Guidance
- Frontend
  - Unit: Vitest + Testing Library. Prefer units for speed and coverage.
  - E2E: Playwright. Bring the stack up (`make up`) before running `pnpm -C alt-frontend test:e2e`.
- Backend
  - Use Go `testing` with table-driven cases. Place tests next to code.
  - Use gomock fakes in `alt-backend/app/mocks`; generate via `make generate-mocks`.
- Coverage
  - Frontend: `pnpm -C alt-frontend test:coverage`
  - Backend: `go test -cover ./...` (from `alt-backend/app`)

## Service Playbooks

### Frontend (Next.js)
- Add a component
  - Create under `alt-frontend/src/components/ComponentName/` or appropriate feature folder.
  - Export via an `index.ts` if used across modules.
  - Add unit tests next to the component or under `tests/` with `*.test.tsx`.
  - Run `pnpm -C alt-frontend test` and lint/format scripts.
- Add a page/route
  - Follow Next.js routing conventions under `src/app` or `src/pages` (depending on setup).
  - Co-locate tests and minimal integration tests where helpful.

### Backend (Go Clean Architecture)
- Add a domain type
  - Add types/interfaces in `alt-backend/app/domain/...`; keep domain free of infra concerns.
  - Add focused unit tests alongside.
- Add a use case
  - Implement in `alt-backend/app/usecase/...`; depend on domain interfaces.
  - Provide table-driven tests; mock gateways via gomock.
- Add an adapter or gateway
  - Outbound/inbound adapters in `gateway/` or `driver/` as appropriate.
  - Keep `rest/` for HTTP handlers; wire dependencies via constructors.
  - Add handler tests and happy-path + edge-case coverage.

### Other Services
- Follow the same principles: keep core logic testable, infra concerns isolated, and tests close to code.

## Local Dev & Docker
- Environment
  - `make up` auto-creates `.env` from `.env.template` if missing.
  - Use `make down`/`make down-volumes` to clean; beware destructive actions.
- SSL for local DB
  - Use the provided make targets to set up and verify. Clean SSL artifacts with `make dev-clean-ssl` when done.

## Skaffold & Kubernetes
- Overview
  - Multi-layer Skaffold setup lives under `skaffold/` with an orchestrator at `skaffold/skaffold.yaml` using `requires` to chain layers.
  - Layers: `01-foundation` (cert-manager, config, secrets, network policies), `02-infrastructure` (Postgres, ClickHouse, Meilisearch), `04-core-services` (backend, proxies), `05-auth-platform` (Kratos, auth-service), `06-application` (frontend, ingress), `07-processing` (jobs/cron services), `08-operations` (monitoring, backups).
  - Profiles: `dev`, `staging`, `prod` (activate with `-p <name>`). Some layers only define `prod`.

- Common Commands
  - Orchestrated dev (all required layers): `skaffold dev -p dev -f skaffold/skaffold.yaml`
  - Orchestrated deploy once: `skaffold run -p dev -f skaffold/skaffold.yaml`
  - Specific layer deploy (e.g., infrastructure): `skaffold run -p dev -f skaffold/02-infrastructure/skaffold.yaml`
  - Rebuild + redeploy a service (watch): use `skaffold dev` in the layer that builds that image.

- Local Cluster Assumptions
  - Optimized for kind/minikube with local images (many profiles set `image.pullPolicy: Never`).
  - Ensure Docker sees the same daemon as your cluster. For kind: load images via Skaffold or `kind load docker-image` if needed.
  - Cert-manager CRDs and namespaces are created by `01-foundation` in `dev`.

- Namespaces & Components
  - `alt-config`, `alt-apps`, `alt-auth`, `alt-database`, `alt-search`, `alt-analytics`, `alt-processing`, `alt-ingress`, `cert-manager`.
  - Examples: backend in `alt-apps`; Postgres in `alt-database`; Meilisearch in `alt-search`; ClickHouse in `alt-analytics`; auth stack in `alt-auth`.

- Profiles & Images
  - Image templates derive from Skaffold variables like `{{.IMAGE_REPO_*}}` and `{{.IMAGE_TAG_*}}` per artifact; dev profiles favor local repositories.
  - `06-application` `dev` builds `alt-frontend` locally; `prod` references `kaikei/alt-frontend` with prod-specific env args.
  - `02-infrastructure` wires Atlas migration image into the Postgres chart via `setValueTemplates`.

- Verification
  - Pods: `kubectl get pods -A --field-selector=status.phase!=Succeeded`
  - Status: `kubectl get deploy,statefulset,job -A -o wide`
  - Logs: `kubectl logs -n <ns> <pod> --tail=100`
  - Events: `kubectl get events -A --sort-by=.lastTimestamp | tail -50`
  - Rollouts: `kubectl rollout status deploy/<name> -n <ns>`

- Troubleshooting
  - Pull policy Never: If images aren’t found, ensure Skaffold built them or switch to a profile that pushes to a registry the cluster can pull from.
  - CRDs missing: Re-run `skaffold run -p dev -f skaffold/01-foundation/skaffold.yaml` to install cert-manager and configs.
  - Helm waits/timeouts: Orchestrator uses `--atomic --wait --timeout`; check `kubectl describe` for blocking conditions (PVCs, Webhooks, PSS).
  - Security/PSS: Orchestrator prints Pod Security labels and warnings; fix violations or use appropriate namespaces.

- Argo CD
  - Repo includes `argocd/` as a placeholder for GitOps configs; not wired into Skaffold. If adopting Argo CD, mirror charts/releases there and keep values in sync.

## Security & Secrets
- Never commit secrets, tokens, or local logs.
- Keep `.env` local; base it on `.env.template`.
- Treat sample credentials as placeholders only.

## Commit & PR Guidance
- Commits (for humans): Imperative mood, concise scope first line (e.g., `Fix: …`, `Refactor: …`); group related changes.
- PRs: Include description, linked issues, reproduction steps, and screenshots for UI changes. Ensure CI (Go + frontend units/E2E + quality gates) is green.
- Agents: Do not commit by default; offer to prepare a commit if requested.

## Troubleshooting
- Compose fails to start
  - Ensure Docker is running, `.env` exists, and ports are free.
  - Rebuild images if needed (`make clean` then `make up`).
- Frontend E2E issues
  - Ensure services are up via `make up` and that Playwright has the required browsers installed locally.
- Backend test flakes
  - Run with `-race -count=1`; isolate packages to find offenders.
- SSL errors
  - Re-run `make dev-ssl-setup` and verify with `make dev-ssl-test`.

## Agent Checklist (Before Hand-off)
- Plan updated; all steps either completed or clearly marked.
- Changes minimal and scoped; no unrelated refactors included.
- Code compiles; relevant tests pass locally.
- Formatting/linting run on affected parts.
- Final message summarizes changes and proposes optional next steps.

## Glossary
- Domain: Business entities and rules; infra-agnostic.
- Use Case: Application-specific orchestration of domain logic.
- Adapter/Gateway: Integration with external systems or inbound interfaces.
- Driver: Framework/IO side initiating calls into the application.
