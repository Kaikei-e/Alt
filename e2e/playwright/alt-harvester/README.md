# alt-harvester — Playwright API E2E suite

End-to-end scenarios for the job-runner third of the alt-backend three-binary
split (ADR-000954). `cmd/harvester` owns the schedules
`job.RegisterHarvesterJobs` wires up — hourly feed collection, the outbox
worker, the daily scraping-policy refresh, OG-image retention, outbox pruning —
and **serves no API**. The only socket it opens is the operator listener on
`:9110`: `/health` for the compose probe, `/metrics` for the `alt-harvester`
scrape job in `observability/prometheus/prometheus.yml`. That port is not
published to the host.

So this is the fleet's one inverted suite. Two tests assert what the service
does; seventy assert what it does not, and the two positives exist to keep the
seventy honest.

## Running

```bash
# Full suite: bring the slice up, run, tear down
bash e2e/playwright/alt-harvester/run.sh

# Build the images from the working tree first — run.sh assumes they exist
python3 e2e/playwright/_lib/build-images.py alt-harvester

# A subset
PW_GREP='@smoke' bash e2e/playwright/alt-harvester/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/playwright/alt-harvester/run.sh
```

Reports land under `e2e/reports/alt-harvester-<RUN_ID>/`.

### The stack

`run.sh` brings up five compose services from the `alt-harvester` profile of
`compose/compose.staging.yaml`:

| Service | Why it is in the slice |
|---|---|
| `alt-backend-db` | Postgres. Nothing in this suite touches it directly. |
| `alt-backend-db-migrator` | Atlas. alt-data-hub will not start against an unmigrated schema. |
| `alt-backend-deps-stub` | Every upstream alias the jobs call resolves here. |
| `alt-data-hub` | The **real** data plane. After Wave 3 `cmd/harvester` holds no DB pool, and `di.newDataHubClient` panics at boot without a reachable configuration for it. |
| `alt-harvester` | The service under test. |

`suite_pki alt-data-hub alt-harvester` mints throwaway mutual-TLS material per
run: one CA, a server leaf for alt-data-hub and a client leaf named
`alt-harvester`, which is what `DATAHUB_ALLOWED_PEERS` admits. It must run
before `suite_up` — the Go client reads its material at construction, so
certificates minted afterwards are certificates the process never saw.

### Iterating without Docker

```bash
cd e2e/playwright && npx tsc --noEmit -p alt-harvester/tsconfig.json
cd alt-harvester && OPS_URL=http://alt-harvester:9110 HARVESTER_HOST=alt-harvester \
  npx playwright test --list
```

## What the suite holds

| Claim | Where |
|---|---|
| `:9110/health` answers `{"status":"healthy","service":"alt-harvester"}` and nothing else | `tests/ops-surface.spec.ts` |
| `:9110/metrics` is a live Prometheus scrape, not the unwired-exporter 503 | `tests/ops-surface.spec.ts` |
| The user-facing REST API is absent | `tests/absent-surfaces.spec.ts` |
| Every Connect service `connect/v2/server.go` mounts is absent | `tests/absent-surfaces.spec.ts` |
| The data plane and the retired `services.backend.v1` / `/v1/internal/*` names are absent | `tests/absent-surfaces.spec.ts` |
| The admin Connect services are absent | `tests/absent-surfaces.spec.ts` |
| `net/http/pprof` and `expvar` are not published | `tests/absent-surfaces.spec.ts` |
| Nothing is bound on `:9000` / `:9101` / `:9102` / `:9443` / `:9103` | `tests/topology.spec.ts` |

Every path assertion is **404**, never 401/403. A 401 would mean the handler is
compiled in and registered here with only a middleware between it and the
caller; 404 says the surface does not exist in this binary — which for
alt-harvester is a compile-time fact, because `di/container_harvester.go`
builds no handler at all.

The closed-port checks matter separately from the 404s: a 404 from a bound port
still means the harvester is listening for RPC.

## Retired Hurl scenarios

| Hurl file | Landed in |
|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` — the readiness gate. It was never a test: a readiness check inside the suite is order-dependent by construction, and `fullyParallel` has no notion of "run this one first". Its two `jsonpath` assertions survive as the `opsHealthSmokeSchema` the gate polls for, and again as real tests in `tests/ops-surface.spec.ts`. |
| `01-operator-surface-only.hurl` (positives) | `tests/ops-surface.spec.ts` — the `/health` envelope, its content type, and `/metrics`. |
| `01-operator-surface-only.hurl` (404s) | `tests/absent-surfaces.spec.ts`. |
| `run.sh` → `assert_transport_refused` loop | `tests/topology.spec.ts`. Four ports of coverage that lived in a bash `for` loop, invisible to the JUnit report and unreachable by `--grep`, are now five named tests. |

Nothing was dropped.

## What is stronger than the Hurl suite was

**`jsonpath "$.status" == "healthy"` became a schema.** Two independent
JSONPath assertions say nothing about the envelope as a whole. `opsHealthSchema`
is `strict()` — the only strict schema in the fleet — because this listener
authenticates nobody and is reachable by every container on the staging
network, so a field added here is a field published to all of them. The
`service` literal is also the only thing in the response that distinguishes the
three binaries of the split, which makes it the one assertion that can catch a
slice built with the wrong `--build-arg BINARY=`.

**The Connect probe went from a sample to the whole list.** Hurl probed one
user-facing service, two DataHubService procedures and two admin procedures.
That is enough to catch "someone mounted the whole router here" and not enough
to catch "someone mounted *one* service here" — which is the far likelier
mistake, since `di.NewHarvesterComponents` already links `datahubv1connect` and
re-registering the server side costs one line. The lists now name every service
`connect/v2/server.go` registers.

**The 404s are pinned to Go's `http.NotFound`.** `text/plain`,
`X-Content-Type-Options: nosniff`, body `404 page not found`. That is what makes
each 404 a topology fact rather than a business one: any framework, proxy or
error handler in front of this mux would answer JSON.

**The closed-port assertions are tests.** Hurl cannot express "the connection
must not be established" — an entry that fails to reach the server is a run
failure. The old suite inverted the polarity outside the framework with a probe
file that passed on any response plus a shell wrapper demanding exit code 3.
`_shared/net.ts` keeps the property that inversion existed to protect: it
matches the error *text*, so a broken probe fails rather than reading as "the
boundary is closed".

## New coverage, and what grounds it

| Added | Grounded in |
|---|---|
| `/debug/pprof/*` and `/debug/vars` are 404 | `internal/bootstrap/ops.go` builds an explicit `http.NewServeMux()` and says why: `http.DefaultServeMux` is where `net/http/pprof` registers itself from an `init()`. The comment was the intent; these are the enforcement. |
| `:9103` is closed | The same file records that `cmd/harvester` used to open `HARVESTER_HEALTH_ADDR` on `:9103` before `OPS_LISTEN` replaced the three per-binary health ports. Two health surfaces disagreeing about one process is how "every container unhealthy, every scrape target down" happened the first time. |
| `/metrics` publishes named metric families | `utils/otel/metrics.go` serves `promhttp.Handler()` over `prometheus.DefaultGatherer`, which carries the Go and promhttp collectors unconditionally. `body contains "# HELP"` is satisfied by one stray HELP line. |
| `/metrics` is not the 503 fallback | `NewOpsHandler` registers `/metrics` either way, answering 503 with prose when OTel produced no exporter. "The path resolves" therefore proves nothing about observability. |
| `/metrics` is gathered per scrape | `promhttp_metric_handler_requests_total` must rise between two scrapes. A snapshot taken at startup would satisfy every other assertion here and give Prometheus a flat line forever. |
| `GET /` and `GET /health/` are 404 | Go 1.22 patterns register exact matches, not subtrees, and nothing registers `/`. This makes "exactly two routes" literal. |
| `/v1/dashboard/jobs` is 404 | It is the admin view *of the jobs this binary runs*, behind `RequireAuth + RequireAdmin` on alt-backend — the single most tempting route to add here "since the data is local". |
| `ClaimOutboxBatch` / `MarkOutboxProcessed` are 404 | outbox-worker is their only legitimate caller and it lives in this process. Reachable here, anything on the container network could mark an event PROCESSED that was never published — a silent append-first violation. |
| `POST /security/csp-report`, `POST /sse/v1/rag/answer` | Registered on the Echo root rather than the `/v1` group, so a probe that only walked `/v1` would miss exactly the routes someone adds outside the convention. |
| Ops responses carry no `Server` banner | `NewOpsHandler` installs no middleware, so a banner would mean something was put in front of a port whose entire access control is "not published". |

## Parallelism

`workers: 4`, and there is no `workers: 1` project.

The ceiling is not a database — `di.NewHarvesterComponents` has no
`AltDBRepository`, and `cmd/harvester`'s dependency graph contains neither
`alt_db` nor pgx (`di/import_boundary_test.go`). It is not a rate limiter
either; the ops mux has no middleware. What the listener does share is a
process with five scheduler goroutines, one of which (`outbox-worker`) ticks
every five seconds. 4 keeps the suite fast without starving that tick on a
two-core runner.

The Hurl runner used `--jobs 1`, and its own comment says why: "the suite is
read-only and short; serial keeps a broken run readable in order". That is a
statement about output, not about correctness, and nothing is lost by dropping
it. No test here can mutate anything — the binary exposes nothing that writes.

## What this suite does not cover

That the schedules actually fire. Job timing is not an HTTP fact, and an E2E
that waits for a cron tick buys flakiness rather than coverage. Job behaviour
stays where it lives: unit tests under `alt-backend/app/orchestrator/job/`, and
— for any job that produces a cross-service event, which `outbox-worker` does —
a Pact CDC test, per CLAUDE.md rule 7.

Nor does it cover the two image jobs. `IMAGE_PROXY_ENABLED=false` in the
staging slice, so `registerImageJobs` deliberately registers neither rather than
ticking a no-op — which is the correct Rule 8 shape, and is observable in the
startup log rather than over HTTP.
