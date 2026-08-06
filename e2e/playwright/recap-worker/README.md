# recap-worker — Playwright API E2E

End-to-end gate for `recap-worker`'s HTTP surface and for the recap pipeline
end to end. Replaces `e2e/hurl/recap-worker/` (ADR-000763 / 000765 / 000766).

## The stack

recap-worker is a Rust axum service with **one plaintext listener on :9005**
and a second rustls listener on :9443 that this slice deliberately leaves
unbound. It hosts no Connect-RPC service of its own — it is a Connect *client*
of alt-data-hub — so there is no `connect-surface.spec.ts` here; the equivalent
"is this handler actually registered" check is `expectRouteMounted` in
`src/assertions.ts`, which discriminates 404 (route gone) and 405 (get/post
swapped) from a real answer.

```
recap-db ── recap-db-migrator (one-shot Atlas apply)
    │
recap-worker :9005  ──►  recap-pipeline-stub  (one container, four aliases)
                            rw-stub-subworker
                            rw-stub-news-creator
                            rw-stub-alt-backend
                            rw-stub-tag-generator
```

The real `recap-subworker` and `news-creator` are GPU-bound; the stub
(`e2e/stubs/recap-pipeline-stub/main.py`) serves schema-valid canned responses
for every RPC the pipeline issues, so a trigger genuinely walks
`ListRecapArticles → classify-runs → clustering → summary/generate →
recap_outputs` in a few seconds.

The stub also answers **200 to every unmatched path**, which is why
`tests/topology.spec.ts` pins `GET /health` as a 404: recap-worker registers
`/health/live` and `/health/ready` and no bare `/health`, so that single
request is the only one that tells the two containers apart.

## Running it

```bash
# The images must exist first, or you are testing the previous build.
python3 e2e/playwright/_lib/build-images.py recap-worker

# recap-worker also needs the rust-bert cache warmed on the HOST: compose
# bind-mounts /opt/rustbert-cache read-only and alt-staging has no egress.
sudo mkdir -p /opt/rustbert-cache && sudo chown -R 999:999 /opt/rustbert-cache
docker run --rm -v /opt/rustbert-cache:/opt/rustbert-cache \
  ghcr.io/kaikei-e/alt-recap-worker:ci warmup

bash e2e/playwright/recap-worker/run.sh          # stack up, tests, teardown
KEEP_STACK=1 bash e2e/playwright/recap-worker/run.sh
PW_GREP='@smoke' bash e2e/playwright/recap-worker/run.sh
```

Without a Docker daemon:

```bash
cd e2e/playwright && npx tsc --noEmit -p recap-worker/tsconfig.json
cd recap-worker && BASE_URL=http://x:9005 MTLS_URL=https://x:9443 \
  DEFAULT_GENRE=ai npx playwright test --list
```

## Projects — read this before adding a spec

recap-worker has two kinds of state that naming cannot isolate, and they need
opposite treatment. Hence three projects rather than the usual one.

| Project | Workers | Holds | Why |
|---|---|---|---|
| `cold-start` | 1 | `tests/cold-start.spec.ts` | The empty-database observations. `get_latest_completed_job` and friends take no tenant, owner or name predicate, so "no recap exists yet" is a global singleton observable once per stack. Every other project `dependsOn` this one. |
| `api` | 4 | everything else | Health, metrics, validation, path parameters, the read surface, the dashboard, topology. Nothing written here is read anywhere else. |
| `pipeline` | 1 | `tests/pipeline.spec.ts` | `Scheduler::run_job` holds a `Semaphore(1)` and **rejects** a concurrent run while the handler has already answered 202 (`scheduler/jobs.rs:214-227` + `api/generate.rs:70-84`). Two overlapping triggers both look successful and only one produces anything. Also `mode: "serial"`, and each test drains the lock before it ends. |

The Hurl suite expressed all of this as one `--jobs 1` over the whole run.
Here it is scoped to the eleven scenarios that genuinely need it, and the
other fifty-nine run in parallel.

Worker count is sized against the **DB pool** (`RECAP_DB_MAX_CONNECTIONS`
defaults to 50, `config.rs:1029`, not overridden in the slice) — which is
nowhere near a ceiling, so 4 is a deliberate under-shoot: recap-worker has no
tenant dimension, most reads hit one shared table, and the same tokio runtime
is running an ML pipeline in the `pipeline` project.

## Where each retired Hurl scenario landed

| Hurl file | Landed in | Note |
|---|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` | A readiness gate inside the suite is order-dependent by construction; `fullyParallel` has no "run this first". Widened from `/health/live` alone to live + ready + a DB-backed probe, because nothing in the health path touches Postgres (`connect_lazy`) and a failed migration otherwise surfaces as a 500 in a random spec. |
| `01-health-schema.hurl` | `tests/health.spec.ts` → "health probes" | `jsonpath "$.status" == "live"` became a `.strict()` schema: `HealthReport` shares one struct with the degraded path, and a live response that grew a `detail` key would mean the dependency-free probe started reporting on a dependency. Plus a new test that the two routes are genuinely distinct handlers. |
| `02-metrics-shape.hurl` | `tests/health.spec.ts` → "metrics" | Kept at 200 + `text/plain`, deliberately **not** upgraded to `_shared/http.ts`'s `expectPrometheusText`. `Metrics::new` registers into a fresh `Registry` while `render_prometheus` encodes `prometheus::gather()` (the default registry) — the exporter is mute, and asserting `# HELP` would fail. A per-line exposition regex reads like a strengthening, but with an empty body it never runs an iteration — so the emptiness itself is asserted (`src/assertions.ts`), which fails loudly the day the exporter starts publishing and the family-name assertion becomes possible. |
| `03-recaps-empty.hurl` | `tests/cold-start.spec.ts` | Both windows, exact error strings, in the single-worker project that every other project depends on. Widened to the four other empty-database 404s the Hurl suite never looked at (morning letter, pulse, evaluation) and to the all-zero job-stats ledger. |
| `04-trigger-validation.hurl` | `tests/validation.spec.ts` → "trigger validation" | Ported exactly, then mirrored onto `/3days` (both routes funnel through one `trigger_recap`, and nothing asserted that they still do) and joined by the three axum extractor rejections and the 405. |
| `05-trigger-and-poll-7days.hurl` | `tests/pipeline.spec.ts` | `retry: 60 / retry-interval: 1000` became `eventually`. Four `jsonpath … exists` checks became real types: `job_id` a UUID, `executed_at` / `window_start` / `window_end` RFC 3339 instants, `genres` a non-empty list of fully-typed genre objects. `[Captures] job_id` is finally *consumed*: the Hurl scenarios captured it and never compared it with anything. `get_latest_completed_job` has no "give me this job" predicate, but the `pipeline` project is single-worker + serial and drains the lock, so the newest completed job for the window is the one the test just triggered. |
| `06-trigger-and-poll-3days.hurl` | `tests/pipeline.spec.ts` | Same drive, parameterised. **Not** upgraded with a `window_end - window_start == 3 days` check: `get_latest_completed_job` computes both timestamps from the `window_days` literal the handler passed (`store/dao/output.rs:174-177`), so that span equals the requested window for any row it returns and the assertion cannot fail. The job-id correlation above is what catches a `window_days` that stopped propagating — a job filed under the wrong window makes the 3-day fetch 404 or serve someone else's id. |

## What is new

Grounded in the source, not invented. Every addition names its file in a
comment.

- **Listener topology** (`tests/topology.spec.ts`) — nothing bound on
  `MTLS_PORT`; `GET /health` is a 404 (the stub-confusion control); no
  `Server` / `X-Powered-By`; the surface is unauthenticated and a
  peer-identity header is inert.
- **Route mounting** — `expectRouteMounted` on the trigger, the morning
  regenerate and `/admin/jobs/retry`. A `.route(...)` line lost in a rebase is
  invisible to every unit test.
- **`/admin/jobs/retry`** — the rule-8 fence. The old handler answered 202
  whether or not there was anything to retry; this asserts each of the three
  outcomes carries its own status *and* its own body, and that a 202 names the
  failed job it is retrying.
- **`/v1/recaps/genres/indexable`** — the search-indexer contract, including
  the `has_more == (results.length == limit)` page-boundary relation.
- **`/v1/morning/*`** and **`/v1/pulse/latest`** and
  **`/v1/evaluation/genres/*`** — listed as "out of scope (deferred phases)"
  by the Hurl README; every validation and miss branch is now pinned.
- **The whole `/v1/dashboard/*` surface** — seven hand-written SQL queries the
  compiler cannot see and the mock DAO hides, now schema-checked live.
- **The `since` asymmetry** — `/v1/recaps/genres/indexable` silently ignores an
  unparseable cursor (`.ok()`, full-corpus scan, no signal) while
  `/v1/morning/updates` rejects one with 400. Same parameter name, opposite
  behaviour; written down rather than rediscovered.

## Out of scope

- **`POST /v1/morning/letters/regenerate` happy path.** It runs the morning
  pipeline synchronously inside the request. Only its mounting is asserted.
- **`POST /v1/evaluation/genres` happy path.** Running the classifier over the
  real golden dataset is minutes of CPU; only the dataset-load rejection is
  asserted.
- **mTLS-on configuration** (`MTLS_ENFORCE=true` + pki-agent). Staging stays
  plaintext, per the news-creator / tag-generator precedent. This suite asserts
  the *consequence* — port 9443 closed — rather than the enabled path.
- **The JST batch daemon** (02:00 JST) and the morning daemon
  (`MORNING_DAEMON_ENABLED=false` in the slice).
- **Real LLM / SBERT runs.** Would need GPU runners.
