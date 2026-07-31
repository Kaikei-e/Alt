# alt-harvester Hurl E2E suite

End-to-end scenarios for the job-runner third of the alt-backend 3-binary
split. alt-harvester owns the schedules that `job.RegisterAllJobs` wired
into the monolith's `main.go` and serves no API. The only socket it opens is
the operator listener on `:9110` — `/health` for the compose probe,
`/metrics` for the `alt-harvester` scrape job in
`observability/prometheus/prometheus.yml` — and that is not published to the
host.

**Status: wired.** The `alt-harvester` profile exists in
`compose/compose.staging.yaml` and the sibling job exists in
`.github/workflows/e2e-hurl.yml`. The suite was written outside-in ahead of
the split; the list under "What has to exist for this to go green" is now a
description of what is there rather than a to-do.

## The contract this suite exists to hold

| Claim | Where it is asserted |
|---|---|
| `:9110/health` answers `{"status":"healthy","service":"alt-harvester"}` | `00-setup.hurl`, `01-operator-surface-only.hurl` |
| `:9110/metrics` speaks the Prometheus text format | `01-operator-surface-only.hurl` |
| The user-facing API (`/v1/*`, user Connect services) is absent | `01-operator-surface-only.hurl` |
| The data plane (`alt.datahub.v1.DataHubService`, and the retired `services.backend.v1` / `/v1/internal/*` names) is absent | `01-operator-surface-only.hurl` |
| The admin Connect services are absent | `01-operator-surface-only.hurl` |
| Nothing is bound on `:9000` / `:9101` / `:9102` / `:9443` | `run.sh` → `assert_transport_refused` |

Every path assertion is **404**, never 401/403. A 401 would mean the handler
is compiled in and registered here with only a middleware between it and the
caller; 404 says the surface does not exist in this binary. For
alt-harvester that is a compile-time fact —
`di/container_harvester.go` builds no handler at all, so "a job that starts
reaching for one of those fails to compile here rather than silently
no-op'ing at runtime" — and this suite is what keeps it observable from
outside the process.

The closed-port checks matter separately from the 404s: a 404 from a bound
port still means the harvester is listening for RPC.

## What this suite does not cover

That the schedules actually fire. Job timing is not an HTTP fact, and an
E2E that waits for a cron tick buys flakiness rather than coverage. Job
behaviour stays where it already lives: unit tests under
`alt-backend/app/orchestrator/job/`, and — for any job that produces a
cross-service event, which `outbox-worker` does — a Pact CDC test, per
Critical Rule 7.

## Running

```bash
# Full suite (brings up the slice, runs Hurl, tears down)
bash e2e/hurl/alt-harvester/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/hurl/alt-harvester/run.sh

# Reports (JUnit + HTML) land under:
ls e2e/reports/alt-harvester-*/
```

The script expects the alt-harvester image to already exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-alt-harvester:${IMAGE_TAG:-ci}`, built
from `alt-backend/Dockerfile.backend` with `--build-arg BINARY=harvester`
(see `services.yaml`).

## Why the closed-port checks are shell and not Hurl

Hurl cannot express "the connection must not be established": an entry that
fails to reach the server is a run failure, not a passing assertion. The
polarity is inverted in `_lib/probe-transport-refused.hurl` (which passes on
*any* response) plus `_lib/assert-transport-refused.sh` (which requires Hurl
exit code **3** — runtime error — so a parse error or an assert failure
cannot masquerade as a refusal). The alt-data-hub suite uses the same pair.

## What the suite depends on (all present)

1. An `alt-harvester` service on the `alt-harvester` profile in
   `compose/compose.staging.yaml`, sharing `alt-backend-db` /
   `alt-backend-db-migrator` / `alt-backend-deps-stub` with the
   `alt-backend` profile, with:
   - `build.args.BINARY: harvester`
   - the operator listener on `:9110` serving `/health` and `/metrics`
   - **no** listener on `:9000` / `:9101` / `:9102` / `:9443`
   - the scheduler explicitly enabled, and the upstream aliases the jobs
     call pointed at `alt-backend-deps-stub`
2. A healthcheck that works without a shell — the runtime image has none;
   `compose/core.yaml` uses `["CMD", "/app-entry", "healthcheck"]`.
3. A sibling job in `.github/workflows/e2e-hurl.yml` following the existing
   per-service job shape: build with `--build-arg BINARY=harvester`, then
   run this script with `STAGING_PROJECT_NAME: alt-staging-alt-harvester`.
