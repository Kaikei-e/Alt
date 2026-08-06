# mq-hub — Playwright API E2E suite

End-to-end scenarios for mq-hub, Alt's Redis Streams broker: the Connect-RPC
service `services.mqhub.v1.MQHubService`, the hand-rolled `/health` route and
the Prometheus `/metrics` endpoint — all three on the one listener the service
has.

This **replaces** the Hurl suite at `e2e/hurl/mq-hub/`. It keeps the same
dispatch convention ([ADR-000766](../../../docs/ADR/000766.md)): alt-deploy's
`release-deploy.yaml` e2e matrix probes `e2e/<framework>/<svc>/run.sh`, so
`bash e2e/playwright/mq-hub/run.sh` is the contract.

## The stack

Two containers, the `mq-hub` profile of `compose/compose.staging.yaml`:

| Service | What it is |
|---|---|
| `redis-streams` | `redis:8.4.5-alpine`, `--maxmemory 256mb --maxmemory-policy noeviction`. The patch pin is load-bearing: 8.4.0–8.4.3 silently trim nothing when `MAXLEN ~` is combined with `ACKED`, which is exactly what `redis_driver.go` sends. |
| `mq-hub` | The service under test, on `:9500`. `REDIS_POOL_SIZE=20`, `STREAM_MAX_LEN=10000`, and no `MAX_BATCH_SIZE` — so `config.go`'s default of 1000 is what the batch specs assert a boundary at. |

No stubs and no PKI. mq-hub speaks plaintext HTTP only (`main.go` calls
`ListenAndServe`), and `middleware.PeerIdentityMiddleware` — the one component
that would need client certificates — is unwired dead code that the service
announces at startup with a `peer_identity_disabled` warning. `run.sh`
therefore has no `suite_pki` call, and `tests/topology.spec.ts` asserts that
posture instead of assuming it.

There is deliberately **no tag-generator** in this slice. It lives in the
`tag-generator` profile, which is a different suite; without it,
`GenerateTagsForArticle` can only reach its timeout branch, which is what the
Hurl suite tested and what this one tests — more precisely.

## Running

```bash
# Full lifecycle: bring the slice up, install deps, run, tear down
bash e2e/playwright/mq-hub/run.sh

# A subset
PW_GREP='@smoke' bash e2e/playwright/mq-hub/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/playwright/mq-hub/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging-mq-hub \
  exec redis-streams redis-cli --scan --pattern 'alt:events:e2e-*'
```

Reports land in `e2e/reports/mq-hub-<RUN_ID>/` — HTML + JUnit for a plain run,
a blob under `PW_BLOB=1`.

The script expects the image to exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-mq-hub:${IMAGE_TAG:-ci}`. CI builds it in
a preceding step; locally:

```bash
docker build -t ghcr.io/kaikei-e/alt-mq-hub:ci -f mq-hub/Dockerfile mq-hub
```

### Iterating without Docker

Every endpoint is read from the environment, so a run against an
already-running stack needs no container:

```bash
cd e2e/playwright/mq-hub
BASE_URL=http://localhost:19500 OPS_ABSENT_URL=http://localhost:19110 \
  npx playwright test
```

`19500` is the host port `compose.staging.yaml` publishes for mq-hub.

## What changed in the port

### `--jobs 1` is gone

The Hurl runner called it load-bearing, and it was — for Hurl.
`12-stream-info.hurl` asserted `length >= 1` and
`groups[*].name includes hurl-e2e-cg-{{run_id}}` against
`alt:events:articles`, which only held because `04`, `07` and `10` had already
written to that one shared key. Run them in any other order and XINFO answered
500 on a stream that did not exist yet.

Nothing about the RPC required a shared key. `StreamGateway.Publish` only
*warns* for an unrecognised stream name (`stream_gateway.go:31-36`) and
publishes anyway, so the `stream` fixture in `src/fixtures.ts` gives every test
`alt:events:e2e-<testToken>` of its own. The assertions get stronger as a side
effect: "at least one entry, put there by somebody" becomes "exactly the three
this test wrote". One spec still publishes to a canonical key, as the control
that the four `domain.StreamKey` constants keep working.

### The Python fixture generator is gone

`e2e/fixtures/mq-hub/gen-batch-oversize.py` existed to write a 1001-event
JSON document at run time, because Hurl can only send a literal file and the
file was too big to commit. `src/events.ts` builds it in the spec that uses it,
so run.sh needs no Python and there is no gitignored artifact to explain.

The committed fixtures were also frozen in a way that mattered: every run
published the *same* `eventId`, which is the field the proto documents as the
consumer's dedupe key. A builder mints a fresh UUID per call.

### `jsonpath "$.code" exists` became a schema and a code

Five of the fourteen Hurl scenarios asserted a status band of `>= 400 < 600`
plus `$.code exists`. That cannot distinguish `invalid_argument` from
`unavailable` — the exact distinction `handler.go`'s `mapPublishErr` was
written to make, so that a caller knows whether to fix its request or page an
operator. Every one of them now asserts the code, and through
`expectUnaryError` also the HTTP status the Connect protocol pairs with it.

## Where each Hurl scenario landed

| Hurl file | Landed in |
|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` — a readiness gate, not a test. Under `fullyParallel` there is no "run this one first". |
| `01-health-rest.hurl` | `health.spec.ts` › "GET /health reports healthy with Redis connected", "…non-negative uptime" |
| `02-metrics.hurl` | `metrics.spec.ts` › "GET /metrics publishes every mqhub family" |
| `03-healthcheck-rpc.hurl` | `health.spec.ts` › "HealthCheck RPC reports healthy with Redis connected" |
| `04-publish-happy.hurl` | `publish.spec.ts` › "publishes a valid event and returns its stream ID" |
| `05-publish-validation.hurl` | `publish.spec.ts` › "rejects an event with no event_type" |
| `06-publish-nil-event.hurl` | `publish.spec.ts` › "rejects a request with no event at all" |
| `07-publish-batch-happy.hurl` | `publish-batch.spec.ts` › "publishes every event in a valid batch" |
| `08-publish-batch-invalid.hurl` | `publish-batch.spec.ts` › "rejects the whole batch when one event is invalid" |
| `09-publish-batch-oversize.hurl` | `publish-batch.spec.ts` › "rejects a batch of exactly MAX_BATCH_SIZE + 1" |
| `10-consumer-group-create.hurl` | `consumer-group.spec.ts` › "creates a group on a stream" |
| `11-consumer-group-idempotent.hurl` | `consumer-group.spec.ts` › "re-creating the same group succeeds" |
| `12-stream-info.hurl` | `stream-info.spec.ts` › "reports the entries and groups of a stream" |
| `13-stream-info-not-found.hurl` | `stream-info.spec.ts` › "errors on a stream that was never written" |
| `14-generate-tags-timeout.hurl` | `generate-tags.spec.ts` › "times out with deadline_exceeded…", "honours the caller's timeout budget" |

Nothing was dropped. The `[Captures]` block in `04` captured `message_id`
"for audit logging" and no later file read it; the equivalent value is now
asserted in place.

## New coverage

Every addition below is grounded in something in the service's own source; the
file and line are named in the spec.

| Area | What was missing |
|---|---|
| Connect mux registration | Nothing distinguished "this procedure errored" from "this path does not exist". `connect-surface.spec.ts` probes all six against 404 — CLAUDE.md rule 8 at the E2E boundary. |
| `Event.Validate` branches | Three of four required fields (`event_id`, `source`, `created_at`) were never exercised; only `event_type` was. |
| `created_at` substitution | `handler.go:179-182` fills in `time.Now()`, making one of `Validate`'s four branches unreachable from the wire. Now stated. |
| Publish → stream linkage | The response and the stream state were asserted separately and never connected. A handler that answered `success:true` without writing would have passed. |
| Stream-ID monotonicity | Consumers checkpoint by stream ID; a duplicate or a regression silently skips entries. |
| `MAX_BATCH_SIZE` boundary | The suite sent 3 events or 1001. `>` vs `>=` in `publish_usecase.go:103` was undetectable. Both sides are now asserted. |
| No partial persistence | `08`'s README claimed it; the executable never checked. A per-test stream makes it checkable. |
| Empty batch | `redis_driver.go:126` early-returns success. A producer batching on a timer will send one. |
| `startId "$"` | The proto documents two start modes; only `"0"` was tested. The difference shows up in `lastDeliveredId`. |
| `MKSTREAM` | The driver creates the stream with the group, so a consumer can boot before its producer. Never covered. |
| Consumer-group counters | `12` asserted only `groups[*].name`; `consumers`, `pending` and `lastDeliveredId` were untouched. |
| Error-body leakage | `CreateConsumerGroup` deliberately returns a fixed message and logs Redis's text. Now pinned. |
| `radixTreeKeys` / `radixTreeNodes` | Same int64-as-string encoding as `length`, and the fields an operator sizes Redis with. |
| Metric families | The Hurl warmup registered two lazily-created families, so `mqhub_batch_size` and `mqhub_errors_total` could not be asserted. Each is now driven by the test that reads it, and the Redis gauge's **value** is checked, not just its name. |
| Metric labels | `{stream,status}` on the publish counter, and `error_type="batch_too_large"` distinct from `redis_error`. Losing either collapses every dashboard without a unit test failing. |
| Request/reply publish half | `14` asserted only that the call failed — also true of a handler that published nothing and slept. |
| Protocol contract | Unknown procedure (404 plain text, not a Connect envelope), unknown service, `GET` on a unary procedure, malformed JSON, unsupported Content-Type, and that `Connect-Protocol-Version` is not required. |
| Listener topology | Nothing on `:9110`, 404 on unregistered paths, no `Server` header. |
| Access-control posture | mq-hub has no authentication at all and `X-Alt-Peer-Identity` is untrusted input on this listener. Both are now stated as tests rather than left in a README's "out of scope" list. |

### Two negatives worth calling out

`topology.spec.ts` asserts that **nothing answers on `:9110`**, using
`expectConnectionRefused` from `_shared/net.ts`. This is the one thing the
Hurl suite could not express at all: Hurl treats an unreachable server as a
run failure, so the split-topology suites had to invert the polarity outside
the framework with a shell wrapper demanding exit code 3.

`topology.spec.ts` › "a spoofed peer-identity header changes nothing" pins
**current** behaviour, not desired behaviour. `X-Alt-Peer-Identity` is what
`PeerIdentityMiddleware` would *set* from a verified client certificate; since
the middleware is in no chain, it is untrusted input, and the assertion is that
nothing downstream reads it. If a future change starts trusting the header
without an mTLS listener in front to overwrite it, that test breaks — which is
the intended signal.

## Parallelism and isolation

`fullyParallel: true`, four workers. The ceiling is Redis, not CPU: one
single-threaded server behind a 20-connection pool, so four workers keep a
handful of commands in flight and more would only queue.

Isolation is by **naming, not teardown**. Each test gets its own stream key and
consumer-group name from `testToken` (dispatch + worker + random tail), so two
shards on one daemon never see each other's entries. No test deletes anything:
the slice is destroyed with `docker compose down -v` per dispatch, and a
teardown would only race a sibling worker's XINFO.

No `workers: 1` project is needed. The only genuinely global state is the
Prometheus registry, and every metrics assertion is that a family or a labelled
sample is *present* — a monotone property a concurrent worker can only
reinforce.

## Out of scope

- **Async consumption.** Whether pre-processor / search-indexer / tag-generator
  actually consume what mq-hub publishes is covered by the message-pact
  provider verification at `mq-hub/app/driver/contract/provider_test.go`.
- **`GenerateTagsForArticle`'s happy path.** Needs a live tag-generator; it
  lives in the `tag-generator` compose profile and its own suite.
- **Partial batch failure.** `domain.PartialPublishError` is only reachable
  when individual XADDs fail inside a pipeline that otherwise executed — which
  needs Redis to be induced into failing mid-batch (`maxmemory` with a
  denyoom command, say). Unit-tested in `driver/redis_driver_test.go`.
- **The periodic trim pass.** `STREAM_HARD_MAX_LEN` defaults to 50000 with a
  60s interval, so a stream would need 50k entries and a minute of waiting to
  observe it. `usecase/trim_streams_usecase_test.go` covers it.
- **The `timeoutMs` clamp.** `generate_tags_usecase.go` caps a caller's
  timeout at 120s; asserting it costs two minutes of wall clock.
- **Degraded Redis.** Killing `redis-streams` mid-run should flip `/health` to
  `healthy:false` under a 503 and the gauge to 0. The suite lifecycle does not
  stop containers; adding it would mean a serial project of its own.
