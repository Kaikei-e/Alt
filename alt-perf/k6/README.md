# K6 Load Testing for alt-backend

Industry-standard scenario-based load testing using [Grafana K6](https://k6.io/).

## Ethical Constraints (Multi-Layer Defense)

1. **Layer 1 (Endpoint Exclusion)**: Only endpoints accessing local resources (PostgreSQL, Meilisearch) are tested. External API endpoints are excluded.
2. **Layer 2 (10s Cooldown)**: Every scenario iteration ends with `sleep(10)` to rate-limit requests.
3. **Layer 3 (Whitelist)**: Test targets are explicitly defined in `helpers/endpoints.js`. Changes require code review.

## Quick Start

```bash
# Start backend (prerequisite)
docker compose -f compose/compose.yaml -p alt up -d alt-backend

# Smoke test
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/smoke.js

# Load test
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/load.js

# Stress test
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/stress.js

# Soak test (30 min)
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/soak.js

# Spike test
docker compose -f compose/compose.yaml -p alt run --rm k6 run /scripts/scenarios/spike.js
```

## Override VUs / Duration

```bash
docker compose -f compose/compose.yaml -p alt run --rm k6 run \
  --vus 50 --duration 10m /scripts/scenarios/load.js
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `K6_BASE_URL` | `http://alt-backend:9000` | Backend URL (direct, no nginx) |
| `K6_AUTH_SECRET` | (from Docker secret) | `X-Alt-Shared-Secret` value |
| `K6_TEST_USER_ID` | — | Test user UUID |
| `K6_TEST_TENANT_ID` | — | Test tenant UUID |
| `K6_TEST_USER_EMAIL` | — | Test user email |

```bash
# Specify test user
docker compose -f compose/compose.yaml -p alt run --rm \
  -e K6_TEST_USER_ID="<uuid>" k6 run /scripts/scenarios/smoke.js
```

## Scenarios

| Scenario | Executor | VUs | Duration | Purpose |
|----------|----------|-----|----------|---------|
| smoke | constant-vus | 1 | 1m | Endpoint reachability check |
| load | ramping-vus | 0→10→20→0 | 16m | Normal traffic simulation |
| stress | ramping-vus | 0→20→50→100→0 | 19m | Breaking point discovery |
| soak | constant-vus | 10 | 30m | Memory leak / connection exhaustion detection |
| spike | ramping-vus | 5→100→5→0 | 6m | Burst traffic resilience |

## Thresholds

| Scenario | p(50) | p(95) | p(99) | Error Rate |
|----------|-------|-------|-------|------------|
| smoke | — | < 300ms | — | < 1% |
| load / soak | < 200ms | < 500ms | < 1000ms | < 1% |
| stress / spike | — | < 1000ms | — | < 5% |

## Custom Metrics

- `feeds_response_time` — Feed endpoint latency
- `search_response_time` — Search endpoint latency
- `dashboard_response_time` — Dashboard endpoint latency
- `articles_response_time` — Article endpoint latency
- `auth_errors` — 401/403 error count
- `server_errors` — 5xx error count
- `successful_checks` — Check pass rate

## Reports

JSON reports are written to `alt-perf/reports/k6-<timestamp>.json`.

## Directory Structure

```
k6/
  docker-entrypoint.sh      # Secret file → env var, then exec k6
  scenarios/
    smoke.js                 # 1 VU, 1 min
    load.js                  # Ramping 0→20, 16 min
    stress.js                # Ramping 0→100, 19 min
    soak.js                  # 10 VUs, 30 min
    spike.js                 # 5→100→5, 6 min
  helpers/
    config.js                # Environment variable config
    auth.js                  # Auth header construction
    endpoints.js             # Whitelisted endpoint definitions
    checks.js                # Response validation
    metrics.js               # Custom K6 metrics
    summary.js               # handleSummary (JSON report)
  config/
    thresholds.js            # Scenario-specific thresholds
```
