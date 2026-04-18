# recap-worker — Hurl E2E suite

End-to-end gate for `recap-worker`'s HTTP surface and the recap pipeline
end-to-end. Built on the same compose-staging + Hurl harness as
`search-indexer`, `mq-hub`, `knowledge-sovereign`, `tag-generator`, and
`news-creator`. Established conventions are spelled out in ADRs
**000763** (framework), **000764** (Connect-RPC over HTTP/1.1+JSON),
**000765** (DB state machine + `--jobs 1`), **000766** (run.sh dispatch
contract), and **000767** (codec mixing).

## What this suite covers

The recap pipeline involves four upstream services
(`recap-subworker`, `news-creator`, `alt-backend`, `tag-generator`) plus
a Postgres backend (`recap-db`). Real `recap-subworker` and
`news-creator` are GPU-bound and impractical to run in CI. We stand up
a single Python FastAPI **stub container** (`recap-pipeline-stub`) that
joins the `alt-staging` network with **four hostname aliases**
(`rw-stub-subworker`, `rw-stub-news-creator`, `rw-stub-alt-backend`,
`rw-stub-tag-generator` — the `rw-stub-` prefix keeps them disjoint
from the other profiles' real service names so multiple profiles can
share the `alt-staging` network) and serves schema-valid canned
responses for every upstream RPC the worker issues. Atlas migrations
apply to a fresh `recap-db` before the worker boots.

The suite then drives the worker through:

| # | Scenario | What it pins |
|---|----------|--------------|
| 00 | `00-setup.hurl` | `/health/live` retry-gates the suite (dependency-free probe). |
| 01 | `01-health-schema.hurl` | `/health/live` and `/health/ready` schemas; `/health/ready` exercises the stubbed subworker.ping + news_creator.health_check round-trips. |
| 02 | `02-metrics-shape.hurl` | `/metrics` returns Prometheus exposition format with at least one `recap_worker_*` metric. |
| 03 | `03-recaps-empty.hurl` | `GET /v1/recaps/{7,3}days` on an empty `recap_outputs` returns 404 + `{"error":"No <label> recap found"}` (matches `api/fetch.rs:407-418`). |
| 04 | `04-trigger-validation.hurl` | `POST /v1/generate/recaps/7days` with all-empty `genres` returns 400 + `genres array must include at least one non-empty value` (matches `api/generate.rs:53-58` + `normalize_genres`). |
| 05 | `05-trigger-and-poll-7days.hurl` | Trigger 7-day recap → 202 + UUID job_id. Poll `GET /v1/recaps/7days` until populated. Asserts pipeline completion against the stubbed upstreams. |
| 06 | `06-trigger-and-poll-3days.hurl` | Same shape for the 3-day window — proves both windows propagate `window_days` correctly. |

## Codec note

All `recap-worker` HTTP endpoints are axum + serde JSON in **snake_case**
(see `api/health.rs:7-13` `#[serde(rename_all = "snake_case")]`,
`api/fetch.rs:198-206`, `api/generate.rs:11-26`). The `alt-backend`
upstream uses Connect-RPC JSON wire format in **camelCase**
(`clients/alt_backend.rs:14-78`); the stub mirrors that on its
`alt-backend` alias. `news-creator` is FastAPI snake_case +
JSON-Schema-validated (`schema/news_creator.rs::SUMMARY_RESPONSE_SCHEMA`).

## Running locally

```bash
# Boot the stack (--build picks up local stub edits)
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile recap-worker up -d --wait --build \
  recap-db recap-pipeline-stub recap-worker

# Run the suite
bash e2e/hurl/recap-worker/run.sh

# Inspect persisted recaps
docker exec alt-staging-recap-db \
  psql -U recap -d recap \
  -c "select job_id, window_days, status, started_at from recap_jobs order by started_at desc limit 5;"

# Tear down
docker compose -f compose/compose.staging.yaml -p alt-staging down -v
```

`KEEP_STACK=1 bash e2e/hurl/recap-worker/run.sh` leaves the stack up
on exit so you can poke at it.

## Environment overrides

The runner respects the same env contract as the other suites
(ADR-000766):

| Var | Default | Purpose |
|-----|---------|---------|
| `IMAGE_TAG` | `main` | Tag of `ghcr.io/<owner>/alt-recap-worker` and `alt-recap-pipeline-stub`. CI sets `ci`. |
| `GHCR_OWNER` | `kaikei-e` | GHCR namespace. |
| `BASE_URL` | `http://recap-worker:9005` | recap-worker URL as seen from the Hurl container. |
| `HURL_IMAGE` | `ghcr.io/orange-opensource/hurl:7.1.0` | Hurl runner image. |
| `RUN_ID` | `$(date +%s)` | Report directory suffix (`e2e/reports/recap-worker-$RUN_ID`). |
| `KEEP_STACK` | `0` | `1` skips teardown. |

## Out of scope (deferred phases)

- **mTLS-on path** (`MTLS_ENFORCE=true` + pki-agent sidecar). Staging
  stays plaintext per phase precedent.
- **recap-evaluator → recap-worker** direction.
- **Real LLM/SBERT runs** in CI (would need GPU runners).
- **Genre learning, morning letter regenerate, dashboard** endpoints —
  picked up later once the stub harness is proven.
- **Connect-RPC binary protobuf** wire format on the `alt-backend`
  stub — recap-worker's reqwest client uses JSON wire
  (`clients/alt_backend.rs:64-74`), so JSON suffices.
