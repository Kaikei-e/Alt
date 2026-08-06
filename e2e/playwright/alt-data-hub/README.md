# alt-data-hub Playwright E2E suite

End-to-end coverage for the **data plane** of the alt-backend three-binary
split (ADR-000954). `alt-data-hub` serves `services.datahub.v1.DataHubService`
— the surface that authenticates a *service* rather than a user — over mutual
TLS on `:9443`, plus a plaintext operator listener on `:9110`, and it
publishes no host port at all.

The admin Connect services (`KnowledgeHomeAdminService`, `AdminMonitorService`)
did **not** move here; they stay on alt-backend's loopback-bound operator
listener. `di/datahub.NewDataHubComponents` builds no crawler, no search
indexer, no image pipeline and no admin surface.

## The stack

`run.sh` renders an isolated slice of `compose/compose.staging.yaml` and brings
up four services:

| Service | Why it is in the slice |
|---|---|
| `alt-backend-db` | Postgres. alt-data-hub is `alt_db`'s only owner. |
| `alt-backend-db-migrator` | Atlas. A `service_completed_successfully` dependency. |
| `alt-backend-deps-stub` | Answers `AUTH_HUB_URL` / `SOVEREIGN_URL` / `MQHUB_CONNECT_URL`, all of which `config.ValidateDataHubConfig` requires before the process will boot. |
| `alt-data-hub` | The service under test. |

Images: `alt-data-hub` (built from `alt-backend/Dockerfile.backend` with
`--build-arg BINARY=datahub`) and `alt-backend-deps-stub`. See
`e2e/playwright/suites.yaml`.

## Authentication: there is no token

The client certificate **is** the caller's identity. `run.sh` calls
`suite_pki alt-data-hub "$ALLOWED_PEER" "$DENIED_PEER"`, which mints a
throwaway one-day CA, a server leaf for `alt-data-hub` (also copied to the
production filenames `svc-cert.pem` / `svc-key.pem` that compose mounts at
`/certs`), and two client leaves:

| Leaf | Role |
|---|---|
| `${ALLOWED_PEER}.pem` (default `pre-processor`) | In `DATAHUB_ALLOWED_PEERS`. Every positive assertion uses it. |
| `${DENIED_PEER}.pem` (default `rogue-peer`) | Deliberately **not** on the allowlist: a valid chain with the wrong subject. |

`ALLOWED_PEER` is exported before `suite_up` because `compose.staging.yaml`
interpolates it into `DATAHUB_ALLOWED_PEERS` and `docker compose config` bakes
that in when the slice is rendered. If the allowlist name and the leaf name
ever drifted, every positive scenario would die in the TLS handshake with no
assertion to point at the cause.

The material lives under `e2e/fixtures/alt-data-hub/pki/` (gitignored) and
`_lib/suite.sh`'s EXIT trap wipes it, except under `KEEP_STACK=1` — the files
are bind-mounted into a container that is still running.

## Running

```bash
python3 e2e/playwright/_lib/build-images.py alt-data-hub   # build first (CLAUDE.md rule 3)
bash e2e/playwright/alt-data-hub/run.sh

KEEP_STACK=1 bash e2e/playwright/alt-data-hub/run.sh       # leave the stack up
PW_GREP='@smoke'  bash e2e/playwright/alt-data-hub/run.sh  # a subset
```

Checking the suite without a Docker daemon:

```bash
cd e2e/playwright && npx tsc --noEmit -p alt-data-hub/tsconfig.json
cd alt-data-hub && DATA_HUB_URL=… OPS_URL=… PKI_DIR=… ALLOWED_PEER=… DENIED_PEER=… \
  ABSENT_REST_URL=… ABSENT_CONNECT_URL=… ABSENT_OPERATOR_URL=… npx playwright test --list
```

## What the suite covers

| Spec | Claim |
|---|---|
| `health.spec.ts` | Both health surfaces, and that they name *different* things — `connect-rpc` on the mTLS listener (the surface), `alt-data-hub` on `:9110` (the binary). Plus `/metrics` as Prometheus text, which fails on `NewOpsHandler`'s 503 unwired branch. |
| `datahub-service.spec.ts` | `DataHubService` answers over mTLS, with response **bodies** pinned by zod rather than only statuses. |
| `connect-surface.spec.ts` | One procedure per DI capability group in `connect/v2/datahub/server.go`, discriminating 404 (never mounted) and 501 (option left nil) from the business answer. |
| `validation.spec.ts` | Every required-field guard reached, with the Connect code *and* its paired HTTP status; and `not_found` told apart from a missing mount by its envelope. |
| `topology.spec.ts` | The retired names answer nowhere, the ops listener carries nothing else, and `:9000` / `:9101` / `:9102` are not bound. |
| `mtls-boundary.spec.ts` | Anonymous refused, wrong-identity refused, allowed peer accepted — all three at the handshake. |

## Where each retired Hurl scenario landed

| Hurl file | Landed in |
|---|---|
| `00-setup.hurl` — `retry: 60` on `/health` | The **retry** became `setup/global-setup.ts` (a readiness gate in a spec is order-dependent, and `fullyParallel` has no notion of "first"). The **assertions** — `$.status == "healthy"`, `$.service == "connect-rpc"` — became `health.spec.ts` › "mTLS /health names the Connect surface", strengthened to a whole-envelope schema. |
| `01-…-connect-namespace-absent.hurl` — six `services.backend.v1` 404s | `topology.spec.ts` › "the retired Connect namespace answers nowhere", one test per procedure. |
| `01` — the `alt.datahub.v1` alias `HTTP 200` | `topology.spec.ts` › "the ADR-000955 transitional alias", strengthened from "200" to the same schema the canonical name answers, plus two new negatives proving the alias is a prefix rewrite and not a catch-all. |
| `02-retired-internal-rest-absent.hurl` — four `/v1/internal*` 404s | `topology.spec.ts` › "the retired /v1/internal REST pair answers nowhere". |
| `03-ops-listener.hurl` — `/health` + `/metrics` positives | `health.spec.ts`. `$.service == "alt-data-hub"` is now a zod literal, and `# HELP` is `expectPrometheusText`. |
| `03-ops-listener.hurl` — five 404s on `:9110` | `topology.spec.ts` › "the operator listener carries nothing but /health and /metrics", plus new probes for `/v1/internal/*` and `/debug/pprof/*`. |
| `04-datahub-service.hurl` — nine `HTTP 200` probes | `datahub-service.spec.ts`, every one with a schema. `ListRecapArticles` / `GetSystemUser`, which asserted only `status != 404`, moved to `connect-surface.spec.ts` where "not 404" is the point and 501 is also excluded. |
| `04-datahub-service.hurl` — `within_hours: 0` → `400` + `$.code == "invalid_argument"` | `validation.spec.ts` › "ListRecentArticles rejects a non-positive within_hours", via `expectUnaryError`, which asserts the code *and* the status Connect pairs with it. |
| `run.sh` › `assert_transport_refused` × 2 (anonymous, denied peer) | `mtls-boundary.spec.ts`. The shell wrapper existed only because Hurl treats an unreachable server as a run failure; here it is a normal assertion. |
| `run.sh` › `assert_transport_refused` × 3 (`:9000`, `:9101`, `:9102`) | `topology.spec.ts` › "mTLS only means there is no second door", via `expectConnectionRefused`. |

Nothing was dropped.

## What is new

- **Capability-group registration probes.** `connect-surface.spec.ts` walks the
  fourteen `With…` option groups in `server.go`. The optional ones answer
  `501 unimplemented` when their port is nil, which no unit test and no
  previous E2E assertion could see: the process boots, reports healthy, and
  the affected procedures fail silently. This is CLAUDE.md rule 8 at the E2E
  boundary.
- **Twenty-six required-field guards**, each producing `invalid_argument` with
  the correct paired status.
- **`not_found` vs. a missing mount.** Both are HTTP 404 on this listener —
  `cmd/datahub`'s prefix router and connect-go's unknown-procedure path both
  call `http.NotFound`. Only the body tells them apart, so
  `validation.spec.ts` asserts the Connect envelope and
  `connect-surface.spec.ts` asserts the plain text.
- **A positive mTLS control.** The Hurl suite proved two identities were
  refused and never proved one was accepted *with server verification on*.
  `src/mtls-probe.ts` supplies the CA and leaves verification enabled, so the
  suite now also asserts alt-data-hub is serving the leaf this run minted.
- **A 403 is a failure, not a pass.** If the wrong-identity probe ever gets an
  HTTP answer instead of a handshake rejection, `assertRefusedAtHandshake`
  reports it as `tlsutil.WithAllowedPeers`' `VerifyConnection` callback having
  stopped running, with `middleware.RequirePeerIdentity` catching it one layer
  too late. Hurl's exit-code-3 wrapper could not express that distinction.
- **`/debug/pprof` on the ops listener.** `NewOpsHandler` avoids
  `http.DefaultServeMux` precisely so `net/http/pprof`'s `init()` registration
  cannot leak onto an unauthenticated port. That was a comment; it is now a
  test.

## Why there is no `--jobs 1`

The Hurl runner used it, and its own comment says why: "the suite is read-only,
but keeping it serial makes the mTLS handshake failures in a broken run
readable in order." That is a debugging preference, not a data dependency — no
scenario captured a value another consumed. Every test here seeds the name it
reads (`token` in `src/fixtures.ts`, from `_shared/ids.ts`), so the suite runs
`fullyParallel` with four workers.

The worker count is sized against `DB_MAX_CONNS=10` on alt-data-hub — see the
comment in `playwright.config.ts`.

## What this suite still does not cover

- **Zero published ports.** A compose fact, not a runtime one:
  `render-slice.sh` strips every `ports:` key before the staging slice boots,
  so an E2E assertion would pass vacuously. `ZERO_PUBLISH_SERVICES` in
  `scripts/compose-port-audit.py` enforces it.
- **Field-level response contracts for the ~120 procedures.** Those belong to
  the Pact consumer pacts (`pacts/`, `rag-orchestrator/pacts/`), verified
  against the provider by `dataplane/driver/contract/provider_test.go`. This
  suite pins the envelopes the five consumers actually read on an empty
  database, and proves every capability group is wired.
- **Write paths.** `CreateArticle`, `SaveArticleSummary`, `ArchiveArticle`
  and the version-creation procedures are probed only through their validation
  guards. Exercising them for real needs a seeded `feeds` row and would leave
  rows behind for the read assertions above to trip over.
