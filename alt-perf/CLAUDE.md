# alt-perf/CLAUDE.md

## Overview

E2E performance measurement CLI. **Deno 2.x**, Astral browser automation, Core Web Vitals.

> Details: `docs/services/alt-perf.md`

## Commands

```bash
# Test (TDD first)
deno task test

# Scan
deno task perf:scan

# Lint
deno task fmt && deno task lint && deno task check
```

## Core Web Vitals Targets

| Metric | Good | Poor |
|--------|------|------|
| LCP | < 2.5s | > 4.0s |
| INP | < 200ms | > 500ms |
| CLS | < 0.1 | > 0.25 |

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- Use `@std/testing/asserts` for assertions
- Mock Astral browser calls for unit tests
- Use real browser for integration tests

## K6 Load Testing

API-level load tests using Grafana K6. Scripts live in `k6/`.

```bash
# Run via Docker Compose
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/smoke.js
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/load.js
```

- Scenarios: smoke, load, stress, soak, spike
- Targets alt-backend directly (port 9000, no nginx)
- Uses `X-Alt-Shared-Secret` fallback auth
- Whitelist-only endpoints (no external API calls)
- 10-second cooldown per iteration (rate limiting)
- Reports: `reports/k6-*.json`

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Web Vitals Injection**: Inject `web-vitals` library for measurement
3. **Authenticated Tests**: Manage Kratos sessions for protected routes
4. **Output Formats**: Support both JSON and CLI output
5. **K6 Endpoint Safety**: Only whitelist endpoints in `k6/helpers/endpoints.js` — never add external-API-calling endpoints
