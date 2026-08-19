---
title: Deep health contract — P2-11
date: 2026-08-18
tags:
  - runbook
  - health
  - observability
  - alt
---

# Deep health contract

Cheap liveness (`/health`, `/v1/health`, `/healthz`) stays the compose
probe. **Never point a compose `healthcheck` at `/health/deep`.** A deep
probe that fans out to dependencies would restart-loop a live process
whenever an upstream hiccuped.

Wave 5 相当の契約は [[000980]]。手順の詳細は本 runbook。compose probe を `/health/deep` に向けない。

Related: [[ops-surface-budget]] [[synthetic-monitoring]] [[user-journey-slo-gameday]] [[000980]] [[3days-recap-artefact-recovery]] PM-2026-036

## TL;DR

Cheap liveness stays the compose probe. Deep health is **per service** —
do not copy `127.0.0.1:9110/health/deep`. Unpublished PKI ops `:9110` on
news-creator / recap-subworker / search-indexer serves `/health` and
`/metrics` only; `GET /health/deep` is **404**.

```bash
# Cheap liveness — compose / k8s-style probes
curl -sS http://127.0.0.1:9000/v1/health            # alt-backend public
curl -sS http://127.0.0.1:11434/health              # news-creator
curl -sS http://127.0.0.1:8002/health               # recap-subworker
curl -sS http://127.0.0.1:9300/health               # search-indexer

# Deep — published app / ops listeners (copy the row for THAT service)
curl -sS http://127.0.0.1:11434/health/deep         # news-creator FastAPI
curl -sS http://127.0.0.1:8002/health/deep          # recap-subworker
curl -sS http://127.0.0.1:9300/health/deep          # search-indexer REST
curl -sS http://127.0.0.1:9511/health/deep          # knowledge-sovereign ops (host 9511→9501)

# alt-backend deep lives on unpublished ops :9110. Distroless: no wget in
# `docker compose exec`. Probe from a toolbox on the Compose network.
docker run --rm --network alt_alt-network busybox:1.37 \
  wget -qO- http://alt-backend:9110/health/deep
```

Optional in-network probe when the image has curl (Python parents). Go
distroless parents have no curl/wget — use the host bind or the toolbox.

```bash
docker compose -f compose/compose.yaml -p alt exec news-creator \
  curl -sS http://127.0.0.1:11434/health/deep
docker compose -f compose/compose.yaml -p alt exec recap-subworker \
  curl -sS http://127.0.0.1:8002/health/deep
```

## Envelope

JSON object. Status vocabulary is `pass` | `warn` | `fail` (not
`healthy` / `ok` — those remain on the cheap liveness routes).

```json
{
  "status": "pass",
  "service": "alt-backend",
  "checks": [
    {
      "name": "datahub",
      "status": "pass",
      "critical": true,
      "latency_ms": 12
    }
  ],
  "latency_ms": 18,
  "cached": false
}
```

Optional per-check `reason` is an opaque token (`timeout` |
`unavailable` | `not_ready`). It must never carry a DSN, filesystem
path, peer identity, stack trace, URL, or raw error string.

## HTTP mapping

| Overall `status` | HTTP | When |
|---|---|---|
| `pass` | 200 | every check `pass` |
| `warn` | 200 | at least one **non-critical** check is `warn`/`fail`, and no critical check failed |
| `fail` | 503 | any **critical** check is `fail` |

A dependency the process cannot do its job without is critical.
A dependency that degrades a feature is non-critical.

## Execution rules

1. **Parallel checks.** Fan-out; do not serialise independent probes.
2. **Per-check timeout.** Default 250ms. A timeout is `fail` (critical)
   or `warn` (non-critical), never a hang.
3. **Global budget < 1s.** The handler's context deadline is 800ms.
   Compose liveness stays on the cheap route so this budget cannot
   restart the container.
4. **Cache 2–5s + singleflight.** Default TTL 3s. Concurrent callers
   share one in-flight run. Cached answers set `"cached": true`.
5. **No silent 200 on critical failure.** news-creator's cheap `/health`
   may stay 200 while Ollama is down; `/health/deep` must not.

## Placement and abuse

`/health/deep` is **not** on any DoS whitelist. Prefer the listener that
actually implements it (see the table). **PKI ops `:9110` is not a
universal deep-health port.** It serves `/health` + `/metrics` on every
in-process parent; only alt-backend (and knowledge-sovereign's separate
ops `:9501`) also serve `/health/deep` there.

Cheap liveness stays unauthenticated so compose probes keep working.

## Current implementations

| Service | Cheap (compose) | Deep listener | PKI ops `:9110` | Critical checks | Non-critical |
|---|---|---|---|---|---|
| alt-backend | ops `GET /health`; public `GET /v1/health` (liveness only — **does not** claim DB connectivity) | unpublished ops `:9110` `GET /health/deep` (toolbox: `http://alt-backend:9110/health/deep`) | **yes — this is the deep listener** | data-hub mTLS `/health` (the data path) | — |
| news-creator | `:11434` `GET /health` (host `127.0.0.1:11434`) | FastAPI `:11434` `GET /health/deep` | `/health` + `/metrics` only; `/health/deep` → 404 | Ollama `list_models` (the process cannot summarise without it) | — |
| recap-subworker | `:8002` `GET /health` (host `127.0.0.1:8002`) | `:8002` `GET /health/deep` | `/health` + `/metrics` only; `/health/deep` → 404 | classifier / artefact `is_file()` (PM-2026-036 class) | — |
| knowledge-sovereign | `GET /health` on RPC and ops | ops `:9501` `GET /health/deep` (host `127.0.0.1:9511`; admin-token gated when `ADMIN_AUTH` is on) | N/A (ops is `:9501`) | DB ping | projector checkpoint presence |
| search-indexer | `:9300` `GET /health` (host `127.0.0.1:9300`) | REST `:9300` `GET /health/deep` (rate-limited; not skipped) | `/health` + `/metrics` only; `/health/deep` → 404 | Meilisearch health | — |

## Metrics

When a service already exposes Prometheus, deep health may publish:

- `health_deep_status{service=...}` — 0 fail, 1 warn, 2 pass
- `health_deep_latency_seconds{service=...}` — last run (cache hits included)

Journey SLO alert YAML has **landed** (`observability/prometheus/rules/user-journey-slo-alerts.yml`, [[000980]]). Missing series is not a deep-health defect. Synthetic **provider activation** remains the ops gate (`observability/synthetic/probes.yaml` `status: spec_only`).

## What this is not

- Not a substitute for [[synthetic-monitoring]]. In-process deep health
  cannot see nginx, auth-hub, or a browser cookie.
- Not a compose readiness signal. Keep `healthcheck.test` on the cheap
  route.
- Not an invitation to dump dependency URLs into JSON.
