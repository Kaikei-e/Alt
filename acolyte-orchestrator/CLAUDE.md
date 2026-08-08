# acolyte-orchestrator/CLAUDE.md

## Overview

Versioned report generation orchestrator. **Python 3.14+**, **Starlette**, **Connect-RPC**, **LangGraph**.

> Details: `docs/services/acolyte-orchestrator.md`

## Commands

```bash
# Test (TDD first)
uv run pytest

# Unit tests only
uv run pytest tests/unit/ -v

# E2E tests only
uv run pytest tests/e2e/ -v

# Contract tests (Pact CDC)
uv run pytest tests/contract/ -v --no-cov

# Coverage
uv run pytest --cov=acolyte

# Type check
uv run pyrefly check .

# Lint
uv run ruff check && uv run ruff format

# Run
uv run uvicorn main:create_app --factory --host 0.0.0.0 --port 8090
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **E2E**: Full service boot, health check, Connect-RPC round-trip
- **CDC**: Pact consumer tests for acolyte → news-creator, acolyte → search-indexer
- **Unit**: Per-layer tests with mocked dependencies

## Architecture

```
Connect-RPC (AcolyteConnectService) → Usecase → Port ← Gateway → Driver
```

- `handler/connect_service.py` — Connect-RPC `AcolyteService` implementation
- `usecase/graph/` — LangGraph pipeline (planner → gatherer → curator → writer → critic → finalizer)
- `port/` — Protocol interfaces (ReportRepository, LLMProvider, EvidenceProvider, JobQueue)
- `gateway/` — PostgreSQL, news-creator, search-indexer adapters
- `driver/` — DB pool, Connect-RPC client factory

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **No updated_at**: Use integer version_no + change_items for versioning
3. **JSONB for auxiliary only**: Citations, tool traces — NOT core queryable fields
4. **Job queue via FOR UPDATE SKIP LOCKED**: No polling-based race
5. **Evidence hydrate**: Fetch metadata first, body only for top-N
6. **news-creator as inference plane**: Route LLM calls through news-creator semaphore

## Proto Code Generation

```bash
cd proto && buf generate --template buf.gen.acolyte.yaml            # AcolyteService (server)
PATH=/home/koko/go/bin:$PATH \
  buf generate --template buf.gen.datahub-acolyte.yaml              # DataHubService (client)
```

## Notification Outbox

`complete_run` writes a `notification_outbox` row inside the completion
transaction; a relay task forwards it to `DataHubService/EnqueueNotification`
over mTLS. One switch, fail-fast when half-configured:

```
NOTIFICATIONS_ENABLED=true
NOTIFICATION_USER_ID=<kratos identity uuid>   # acolyte-db has no owner column
DATAHUB_URL=https://alt-data-hub:9443
MTLS_ENFORCE=true                             # alt-data-hub always verifies the client cert
```

The relay adds no HTTP route. :8090 authenticates nobody, so it stays `/health`
plus the Connect mount; every tick logs `notification_relay_tick` carrying
`notification_outbox_oldest_pending_age_seconds` and
`notification_outbox_last_tick_timestamp_seconds`, which is the only place
those two values surface here — Prometheus has no series for this service.
