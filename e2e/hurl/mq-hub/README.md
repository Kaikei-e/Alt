# mq-hub — E2E specification

Second Hurl-driven end-to-end suite in the Alt monorepo, following the
search-indexer convention. Each scenario is a paired narrative (this file)
plus an executable `NN-<feature>.hurl`.

mq-hub exposes plain HTTP `/health` + `/metrics` *and* Connect-RPC on the
same `:9500` mux. The RPC surface is driven by Hurl via the **Connect
protocol over HTTP/1.1 with the JSON codec**:
`POST /{service}/{method}` + `Content-Type: application/json` + a proto3-JSON
body. That's a built-in Connect feature — it works without gRPC or h2c.

## Prerequisites

Hurl 7.1.0+ on the host (the repo ships `hurl_7.1.0_amd64.deb`), plus
Python 3 (for the oversize-batch fixture generator) and Docker Compose.

The staging stack must be reachable. The runner brings it up for you:

```sh
bash e2e/hurl/mq-hub/run.sh
```

Or manually:

```sh
IMAGE_TAG=sha-<short> GHCR_OWNER=kaikei-e \
  docker compose -f compose/compose.staging.yaml -p alt-staging \
  up -d --wait redis-streams mq-hub
```

Ports published for host-local testing: `19500:9500` (mq-hub).

## Running

`run.sh` handles the full lifecycle (generate fixture → compose up → Hurl
→ compose down). Override via env:

| Var | Default | Purpose |
|-----|---------|---------|
| `IMAGE_TAG` | `main` | tag of `ghcr.io/<owner>/alt-mq-hub` |
| `GHCR_OWNER` | `kaikei-e` | GHCR namespace |
| `BASE_URL` | `http://localhost:19500` | mq-hub URL (service DNS in-network) |
| `RUN_ID` | `$(date +%s)` | isolates consumer-group names across parallel runs |
| `KEEP_STACK` | `0` | set to `1` to leave the stack up after the run |

Example — debug a failure interactively:

```sh
KEEP_STACK=1 bash e2e/hurl/mq-hub/run.sh   # leave stack up on exit
docker compose -f compose/compose.staging.yaml -p alt-staging \
  exec redis-streams redis-cli XLEN alt:events:articles
```

Reports land under `e2e/reports/mq-hub-<RUN_ID>/` (gitignored):
JUnit XML + HTML.

## Scenarios

### 00 — Readiness probe (pre-flight)
- **Given** the compose healthcheck is converging
- **When** we `GET /health` with `retry:30 retry-interval:500`
- **Then** the response is 200 JSON with `healthy=true`, `redis_status="connected"`
- Executor: `00-setup.hurl`

### 01 — Plain HTTP /health contract
- **Given** mq-hub is ready
- **When** we `GET /health`
- **Then** the body is JSON with `healthy`, `redis_status`, `uptime_seconds`
  (snake_case — this is the hand-rolled handler in `main.go`, not a
  proto-JSON response)
- Executor: `01-health-rest.hurl`

### 02 — Prometheus /metrics surface
- **Given** mq-hub is ready
- **When** we `GET /metrics`
- **Then** the content-type is `text/plain` and the body contains
  `mqhub_publish_total`, `mqhub_publish_duration_seconds`, `mqhub_errors_total`
- Executor: `02-metrics.hurl`

### 03 — Connect-RPC HealthCheck
- **Given** mq-hub is ready
- **When** we POST `{}` to
  `services.mqhub.v1.MQHubService/HealthCheck` with Connect headers
- **Then** the response is 200 JSON with `healthy=true`,
  `redisStatus="connected"`, `uptimeSeconds` numeric (camelCase per
  proto3-JSON canonical form)
- Executor: `03-healthcheck-rpc.hurl`

### 04 — Publish a single event (happy path)
- **Given** a valid `PublishRequest` fixture on `alt:events:articles`
- **When** we POST to `…/Publish`
- **Then** `success=true` and `messageId` matches `^[0-9]+-[0-9]+$`
  (Redis stream ID)
- Executor: `04-publish-happy.hurl`

### 05 — Publish validation (empty event_type)
- **Given** a request whose `event.eventType` is `""`
- **When** we POST to `…/Publish`
- **Then** the server responds with a non-2xx Connect error whose
  `message` references `event_type`
- Why it fails: `stream_gateway.go:40` calls `event.Validate()` and
  returns the bare error upward; Connect wraps it to a non-2xx frame
- Executor: `05-publish-validation.hurl`

### 06 — Publish with the event field omitted
- **Given** a request body `{"stream": "…"}` with no `event`
- **When** we POST to `…/Publish`
- **Then** HTTP 400 with `code="invalid_argument"`
- Why: `handler.go:38` short-circuits with `connect.CodeInvalidArgument`
- Executor: `06-publish-nil-event.hurl`

### 07 — PublishBatch happy path (3 events)
- **Given** three valid events in a `PublishBatchRequest`
- **When** we POST to `…/PublishBatch`
- **Then** `successCount=3` and `messageIds` has three Redis stream IDs
- Note: on the happy path the `errors` and `failureCount` fields are
  absent from the proto3-JSON response (default zero values are omitted)
- Executor: `07-publish-batch-happy.hurl`

### 08 — PublishBatch rejects whole batch when one event is invalid
- **Given** a batch of 3 where event index 2 has empty `eventType`
- **When** we POST to `…/PublishBatch`
- **Then** non-2xx Connect error referencing `event_type`, and **no**
  partial persistence — gateway validates all events before hitting Redis
- Executor: `08-publish-batch-invalid.hurl`

### 09 — PublishBatch oversize rejection
- **Given** a batch of 1001 events (generated at runtime by
  `gen-batch-oversize.py`; the file is gitignored)
- **When** we POST to `…/PublishBatch`
- **Then** non-2xx Connect error whose `message` contains
  `"batch size exceeds"` — usecase rejects before touching Redis
- Executor: `09-publish-batch-oversize.hurl`

### 10 — Create a fresh consumer group
- **Given** a group name scoped to `{{run_id}}`
- **When** we POST to `…/CreateConsumerGroup`
- **Then** `success=true`
- Executor: `10-consumer-group-create.hurl`

### 11 — Consumer-group creation is idempotent
- **Given** the same group name as scenario 10
- **When** we POST to `…/CreateConsumerGroup` again
- **Then** `success=true` — Redis BUSYGROUP is silently absorbed by
  `redis_driver.go:156`
- Executor: `11-consumer-group-idempotent.hurl`

### 12 — GetStreamInfo reflects published state
- **Given** earlier scenarios have populated `alt:events:articles` and
  created the per-run consumer group
- **When** we POST to `…/GetStreamInfo`
- **Then** `length >= 1`, `firstEntryId` / `lastEntryId` match the
  Redis-ID regex, and the `groups[*].name` list includes
  `hurl-e2e-cg-{{run_id}}`
- Executor: `12-stream-info.hurl`

### 13 — GetStreamInfo on a non-existent stream errors
- **Given** a stream name that was never written to
- **When** we POST to `…/GetStreamInfo`
- **Then** non-2xx Connect error — Redis XINFO STREAM returns "no such
  key" which the driver surfaces verbatim
- Executor: `13-stream-info-not-found.hurl`

### 14 — GenerateTagsForArticle times out without a consumer
- **Given** no tag-generator is consuming `alt:events:tags`
- **When** we POST with `timeoutMs=200`
- **Then** non-2xx Connect error within ~1s whose `message` contains
  `"timeout"` — usecase calls `SubscribeWithTimeout`, which returns
  `errors.New("timeout waiting for reply")`
- Executor: `14-generate-tags-timeout.hurl`

## Out of scope for this suite

- **Async event consumption** — whether pre-processor / search-indexer /
  tag-generator actually consume the events mq-hub publishes. That's
  covered by the Pact message-pact provider verification at
  `mq-hub/app/driver/contract/provider_test.go`.
- **GenerateTagsForArticle happy path** — requires a live tag-generator
  (or a stub that reads `alt:events:tags` and writes the reply stream).
  Deferred; add a stub + fixture in a follow-up if needed.
- **mTLS peer-identity enforcement** — `middleware/peer_identity.go`
  exists but `main.go:62` does not wire it onto the main mux. If/when it
  becomes required, add a scenario here that sends an allowed CN header.
- **Redis failure modes** — killing redis-streams mid-test should flip
  `/health` to `healthy=false` and return HTTP 503, but the `run.sh`
  lifecycle doesn't exercise this. Add if we want degraded-state
  coverage.

## Pattern note

Paired Markdown + Hurl gives us a Gauge-style executable spec without
pulling the JVM into the stack. Same convention as
`e2e/hurl/search-indexer/`.
