---
title: Admin Observability UI — Runbook
type: runbook
affected_services:
  - alt-backend
  - alt-butterfly-facade
  - alt-frontend-sv
  - prometheus
  - nginx
owner: platform
last_updated: 2026-04-13
---

# Admin Observability UI — Runbook

The **Observability** tab in the Admin UI surfaces Prometheus-backed metrics via a
Connect-RPC server-streaming RPC (`alt.admin_monitor.v1.AdminMonitorService/Watch`)
that pushes a fresh snapshot every 5 seconds. This runbook covers the
operational knobs and the most likely degraded paths.

## Feature flag

The pipeline is off by default. Enable by setting on `alt-backend`:

```
ADMIN_MONITOR_ENABLED=true
```

`compose/core.yaml` propagates the flag from the host env. Without the flag,
`server.go` skips handler registration and the FE panel shows an empty state.

## Config knobs

All defaults live in `alt-backend/app/config/config.go` → `AdminMonitorConfig`.

| Env | Default | Purpose |
|---|---|---|
| `ADMIN_MONITOR_PROMETHEUS_URL` | `http://prometheus:9090` | Upstream Prometheus base URL. |
| `ADMIN_MONITOR_QUERY_TIMEOUT` | `3s` | Per-query ctx timeout in gateway. |
| `ADMIN_MONITOR_CACHE_TTL` | `10s` | Result cache TTL (instant+range). |
| `ADMIN_MONITOR_RATE_LIMIT_RPS` | `5` | Global rps cap to Prometheus. |
| `ADMIN_MONITOR_RATE_LIMIT_BURST` | `10` | Token bucket burst. |
| `ADMIN_MONITOR_STREAM_INTERVAL` | `5s` | Watch push interval. |

`observability.yaml` additionally constrains Prometheus itself:

```
--query.timeout=5s
--query.max-samples=1000000
--query.max-concurrency=8
```

## Trust boundaries

1. **FE → BFF**: SvelteKit proxy at
   `/api/v2/alt.admin_monitor.v1.AdminMonitorService/*` forwards to
   `alt-butterfly-facade:9250` with the caller's `X-Alt-Backend-Token`.
2. **BFF (`AdminMonitorProxyHandler`)**: validates the JWT, enforces
   `role == "admin"`, swaps user token for the service token, relays
   with chunked flushing.
3. **alt-backend Connect handler**: resolves `MetricKey` via the
   compile-time allowlist. Raw PromQL is never accepted. Out-of-range
   `window`/`step` enums → `CodeInvalidArgument`.
4. **Gateway**: enforces cache + singleflight + global rate limit
   before hitting Prometheus.

## Path anatomy

```
Browser
  └─ /api/v2/alt.admin_monitor.v1.AdminMonitorService/Watch
       └─ SvelteKit route (+server.ts)
            └─ alt-butterfly-facade:9250 (AdminMonitorProxyHandler)
                 └─ alt-backend:9101 (Connect-RPC, admin_monitor.Handler)
                      └─ admin_metrics_gateway
                           └─ prometheus_client
                                └─ prometheus:9090/api/v1/query{,_range}
```

nginx location `^/api/v2/alt\.admin_monitor\.v1\..+` disables
`proxy_buffering` and `proxy_request_buffering`, giving the server stream an
unbuffered path end-to-end. `X-Accel-Buffering: no` is set both at the
alt-backend response and at the SvelteKit proxy as defense-in-depth.

## Most likely failure modes

### Panel shows `degraded` banner and metric rows are `—`

Prometheus is unreachable from alt-backend.

1. `docker compose ps prometheus` → status/healthcheck.
2. `docker compose logs prometheus --tail=100` for `--query.max-samples`
   rejections or OOMs.
3. `curl http://<host>:9090/-/ready` from inside the docker network.
4. The gateway returns `degraded=true` (HTTP 200) rather than erroring,
   so the FE stays alive and will recover automatically when Prometheus
   returns.

### 5-second stream stops updating

Usually an idle kill at an intermediate proxy.

1. In DevTools network tab, confirm the Watch request is still open
   (pending indefinitely with chunked frames).
2. If it closed, check `proxy_read_timeout` at nginx for the
   `alt.admin_monitor.v1` location block (must be ≥ stream rotate
   horizon, currently 1h).
3. FE rotates each stream every 15 min ±60s to prevent idle kills;
   reconnects use exponential backoff with full jitter (1s → 30s cap).

### `CodeInvalidArgument` on Snapshot

FE sent a `window`/`step` not in the allowlist. Accepted values:

- window ∈ `{5m, 15m, 1h, 6h, 24h}`
- step ∈ `{15s, 30s, 1m, 5m}`
- `window/step ≤ 720` (point count guard)

### Non-admin users receive 403

Expected. Both BFF and alt-backend enforce `role == "admin"`. Grant the
role via Kratos traits before testing.

### `auth-hub` / `rask-log-aggregator` show "Not instrumented"

These services do not yet expose `/metrics`. Add blackbox-exporter probes
or native instrumentation in a follow-up; in the meantime the UI shows
the bare "Not instrumented" badge instead of false-positive "down".

## Adding a new metric

1. Append a `domain.MetricKey` constant and an `allowEntry` in
   `alt-backend/app/gateway/admin_metrics_gateway/allowlist.go`.
2. Keep PromQL as a literal string on the entry — do not accept any
   client-supplied fragment.
3. Extend the FE `DEFAULT_KEYS` and add a `MetricRow` in
   `ObservabilityPanel.svelte` if it should render by default.
4. Add a row to the metrics table at the top of this document.

## Allowlist (2026-04-13 refresh — actual metric names)

| key | PromQL | unit |
|---|---|---|
| `availability_services` | `up{job=~"alt-backend\|pre-processor\|mq-hub\|recap-worker\|recap-subworker\|cadvisor\|nginx\|prometheus"}` | bool |
| `http_latency_p95` | `histogram_quantile(0.95, sum by (job,le) (rate(http_request_duration_seconds_bucket[5m])))` | seconds |
| `http_rps` | `sum by (job) (rate(http_requests_total[1m]))` | req/s |
| `http_error_ratio` | `sum by (job) (rate(http_requests_total{status=~"5.."}[5m])) / clamp_min(sum by (job) (rate(http_requests_total[5m])), 1e-9)` | ratio |
| `cpu_saturation` | `sum by (name) (rate(container_cpu_usage_seconds_total{name=~".+"}[2m]))` | cores |
| `memory_rss` | `sum by (name) (container_memory_rss{name=~".+"})` | bytes |
| `mqhub_publish_rate` | `sum by (topic) (rate(mqhub_publish_total[1m]))` | msg/s |
| `mqhub_redis` | `mqhub_redis_connection_status` | bool |
| `recap_db_pool_in_use` | `sum by (pool) (recap_db_pool_checked_out)` | conns |
| `recap_worker_rss` | `recap_worker_rss_bytes` | bytes |
| `recap_request_p95` | `histogram_quantile(0.95, sum by (le) (rate(recap_request_process_seconds_bucket[5m])))` | seconds |
| `recap_subworker_admin_success` | `sum by (status) (rate(recap_subworker_admin_job_status_total[5m]))` | jobs/s |

## Visual contract (Alt-Paper editorial palette)

Global Alt-Paper theme (`src/app.css :root, [data-style="alt-paper"]`) ships
editorial-ink semantic colors that meet WCAG AA on cream `#faf9f7`:

| token | HEX | contrast |
|---|---|---|
| `--alt-success` | `#2f6b3a` | 5.73:1 |
| `--alt-warning` | `#8a5a00` | 6.12:1 |
| `--alt-error`   | `#8c1d1d` | 8.29:1 |

Observability scoped tokens (`ObservabilityPanel.svelte`) add:

- `--obs-spark-stroke: #3b3630` — 10.5:1 sparkline primary
- `--obs-spark-threshold: var(--obs-warn)` — dashed threshold
- `--obs-series-1..4` — Okabe–Ito palette, colorblind-safe
- `--obs-rule` / `--obs-rule-strong` — decorative hairlines only

Pure-yellow `#ffff00` / pure-red `#ff0000` from the global base are intentionally
overridden here because their contrast on cream is 1.07:1 / ~4.0:1 with the
wrong chroma for editorial layout (they read as "neon", not "ink"). Non-Alt-Paper
themes (vaporwave, liquid-beige) keep their own values.

### State rule

Meaning is carried by **text + glyph** (`▲ ▼ ● ○ "up" "down"`) so color is
never the sole channel (WCAG 1.4.1). Sparkline threshold lines are dashed.

## Load check

With cache 10s and stream interval 5s, the gateway issues at most one
upstream query per metric per 10s regardless of concurrent admin
viewers (singleflight). For the default 8-metric set:

- ≈0.8 rps to Prometheus per metric, ≈6.4 rps aggregate, steady-state.
- `--query.max-samples=1000000` bounds a single query's memory.

Exceed at your peril.

## E2E smoke

```bash
docker compose -f compose/compose.yaml -p alt up --build -d \
  alt-backend alt-butterfly-facade alt-frontend-sv prometheus grafana nginx
ADMIN_MONITOR_ENABLED=true \
  docker compose -f compose/compose.yaml -p alt up -d alt-backend

# Open http://localhost in a browser, sign in as admin,
# navigate to /admin/knowledge-home → Observability tab.
# Expect: snapshot updates every ~5s, no visible errors.
```

## Related

- Plan: `/home/koko/.claude/plans/floofy-bubbling-papert.md`
- Proto: `proto/alt/admin_monitor/v1/admin_monitor.proto`
- Allowlist: `alt-backend/app/gateway/admin_metrics_gateway/allowlist.go`
