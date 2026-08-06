# search-indexer — Playwright API E2E suite

End-to-end scenarios for search-indexer, Alt's Meilisearch-backed article
search service: the REST endpoint on `:9300`, the Connect-RPC service
`services.search.v2.SearchService` on `:9301`, and the mutual-TLS mux on
`:9443` that this configuration must **not** bind.

This **replaces** the Hurl suite at `e2e/hurl/search-indexer/`. It keeps the
same dispatch convention ([ADR-000766](../../../docs/ADR/000766.md)):
alt-deploy's `release-deploy.yaml` e2e matrix probes
`e2e/<framework>/<svc>/run.sh`, so `bash e2e/playwright/search-indexer/run.sh`
is the contract.

## The stack

Three containers, the `search-indexer` profile of
`compose/compose.staging.yaml`:

| Service | What it is |
|---|---|
| `meilisearch` | `getmeili/meilisearch:v1.11`, `MEILI_MASTER_KEY=alt-staging-test-master-key`. Holds the `articles` index that search-indexer creates and configures at boot, and the `recaps` index nothing in this slice writes to. |
| `stub-backend` | `nginx:1.27-alpine` answering 200 to everything. search-indexer only probes `BACKEND_API_URL` for reachability at startup and never parses the body. It is also what `RECAP_WORKER_URL` and `REDIS_STREAMS_URL` point at, which is why the recap index stays empty. |
| `search-indexer` | The service under test. `MTLS_LISTEN=false`, `MTLS_ENFORCE=false`, `CONSUMER_ENABLED=false`, hybrid search off (`MEILI_HYBRID_EMBEDDER` unset, so BM25 only and no embedder to warm). |

No PKI. `bootstrap/app.go` binds the `:9443` mutual-TLS mux only when
`MTLS_LISTEN == "true"`, so there is no client certificate to mint —
`tests/topology.spec.ts` asserts the port is closed rather than leaving that an
unstated premise.

## Running

```bash
# Full lifecycle: bring the slice up, install deps, run, tear down
bash e2e/playwright/search-indexer/run.sh

# A subset
PW_GREP='@smoke' bash e2e/playwright/search-indexer/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/playwright/search-indexer/run.sh
curl -s -H 'Authorization: Bearer alt-staging-test-master-key' \
  'http://localhost:17700/indexes/articles/settings' | jq .filterableAttributes
```

Reports land in `e2e/reports/search-indexer-<RUN_ID>/` — HTML + JUnit for a
plain run, a blob under `PW_BLOB=1`.

The script expects the image to exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-search-indexer:${IMAGE_TAG:-ci}`:

```bash
python3 e2e/playwright/_lib/build-images.py search-indexer
```

### Iterating without Docker

Every endpoint is read from the environment, so a run against an
already-running stack needs no container. `17700` and `19300` are the host
ports `compose.staging.yaml` publishes; `:9301` and `:9443` are not published,
so the Connect and topology specs need the in-network form.

```bash
cd e2e/playwright/search-indexer
BASE_URL=http://localhost:19300 \
CONNECT_URL=http://localhost:19301 \
MTLS_ABSENT_URL=http://localhost:19443 \
MEILI_URL=http://localhost:17700 \
MEILI_MASTER_KEY_FILE=../../fixtures/staging-secrets/meili_master_key.txt \
MEILI_SEED_DOCS=../../fixtures/search-indexer/seed-docs.json \
  npx playwright test
```

## Where each Hurl scenario landed

| Hurl file | Landed in |
|---|---|
| `00-seed-meilisearch.hurl` | `setup/global-setup.ts` — a readiness gate and a seed, not a test. Under `fullyParallel` there is no "run this one first". |
| `01-health.hurl` | `health.spec.ts` › "REST :9300 GET /health reports ok" |
| `02-search-basic.hurl` | `search-rest.spec.ts` › "returns the seeded matches with a complete hit envelope", "every hit carries a real ranking score" |
| `03-search-pagination-limit.hurl` | `search-rest.spec.ts` › "limit caps the page and total counts it" |
| `04-search-user-scoped.hurl` | `search-rest.spec.ts` › "user_id=alice sees only their own document", "user_id=bob …", "an unknown user gets an empty result set, not an error" |
| `05-search-empty-query.hurl` | `search-rest.spec.ts` › "q omitted entirely is a 400", "q present but empty is a 400" |
| `06-search-limit-bounds.hurl` | `search-rest.spec.ts` › "a limit that is zero / above the 1000 ceiling / not a number at all falls back to the default of 50" |

Nothing was dropped. The only `[Captures]` block in the suite was
`00-seed-meilisearch.hurl`'s `task_uid`, consumed by the next entry in the same
file; `src/corpus.ts` reads and polls it in the same way, from `globalSetup`.

## What changed in the port

### `jsonpath "$" exists`-class assertions became schemas

`02-search-basic.hurl` probed `hits[0].id exists`, `hits[0].title isString`,
`hits[0].tags isCollection` and so on — five spot checks against **element
zero**, saying nothing about the rest of the page. `searchResponseSchema`
validates every hit, and carries the `total == len(hits)` invariant as a
`refine`, so the REST/Connect divergence that
`03-search-pagination-limit.hurl`'s comment warns about is now enforced on
every response the suite parses rather than in one scenario.

### `score isNumber` became a range

`getFloat64(hit, "_rankingScore")` falls back to `0.0` when the field is
absent. Drop `ShowRankingScore: true` from `newBaseSearchRequest` and every hit
comes back with `score: 0` — still a number, still passing the Hurl assertion,
and silently breaking every consumer that ranks or thresholds on it. The schema
bounds the score to 0..1 and one test asserts it is strictly positive.

### The seed moved, and had to

`driver/meilisearch_cache.go` fronts every search path with a 1024-entry LRU on
a five-minute TTL, and `cache.put` is called unconditionally — **empty result
sets included**. That makes the usual "write, then poll until it appears" shape
actively wrong here: the first poll caches the miss and the next five minutes
of polling are served from that cache no matter what Meilisearch has since
indexed.

So convergence is asserted at the Meilisearch task boundary, before any query
reaches search-indexer: `seedDocuments` blocks on `GET /tasks/{uid}` until it
reports `succeeded`. `globalSetup` does this for the shared corpus and each
worker's `corpus` fixture does it for its own. No spec waits for indexing, and
no spec sleeps.

`globalSetup` also probes search-indexer's `/health` **before** seeding, which
the Hurl pre-step did not: `bootstrap.Run` calls `EnsureIndex` — creating
`articles` with `id` as primary key and `tags` / `user_id` / `published_at` /
`language` filterable — and only then binds its listeners. Seed first and the
documents land in an index Meilisearch auto-created with no filterable
attributes, and every `user_id=` assertion fails in a way that reads like a
query bug.

### Isolation is by naming

The parity scenarios above read the shared fixture corpus, which is seeded once
before any worker starts and is never mutated. Everything else reads a corpus
its own worker seeded: five documents carrying a single-token nonce, under a
`user_id` nothing else owns.

The nonce is pure lowercase letters, which is load-bearing rather than
cosmetic. Meilisearch tokenises on separators, so a hyphenated token becomes
several words — and a multi-word query returns documents matching only *some*
of them, because the `words` ranking rule drops terms. Two workers would then
see each other's documents and every count assertion would be unreliable.

## New coverage

Every addition is grounded in something in the service's own source; the file
is named in the spec.

| Area | What was missing |
|---|---|
| **Connect-RPC on `:9301`** | The Hurl README listed the entire listener as out of scope ("HTTP/2 h2c, not practical to drive from Hurl"). `h2c.NewHandler` passes an ordinary HTTP/1.1 request through to the mux, which is what `APIRequestContext` sends, so all of it is now covered. |
| Connect service registration | Nothing distinguished "this procedure errored" from "this path does not exist" — CLAUDE.md rule 8 at the E2E boundary. Both procedures are probed against 404, with an unknown method and an unknown service as the controls. |
| `SearchRecaps` wiring | `bootstrap.Run` treats a failed `EnsureRecapIndex` as **non-fatal** and leaves `searchRecapsUsecase` nil, which the handler reports as `unimplemented`. "DI never wired it" and "it is intentionally off" are indistinguishable from that alone, so one test asserts it is mounted (400 or 501) and a second asserts it actually answers 200. |
| Connect pagination | `offset` exists only on the RPC. Three pages are walked and asserted disjoint, reassembling into the whole corpus, with a short final page. |
| `estimated_total_hits` | The RPC reports Meilisearch's estimate where REST reports `len(hits)` — the divergence the Hurl README told clients about and never tested. Also the first place the int64-as-**string** protojson encoding is pinned. |
| Omitted `limit` | protojson drops a zero `limit`, and `SearchByUserIDWithPagination` is what turns 0 into 20. Forward the literal 0 to Meilisearch and every caller that omits the field gets zero hits — a total outage under a 200. |
| Tenant isolation | Four negatives, including a query with matches in the index issued by a user who owns none of them, and the same query under two identities back to back — the shape that would surface a cache key that had dropped `user_id`. |
| `published_after` / `published_before` | Never sent by the Hurl suite, and unsendable: its corpus has no `published_at` at all, so a date window would have matched nothing whether the filter worked or not. Both bounds, their inclusivity, their AND, the empty-string form, both parse errors, and the explicit refusal to combine them with `user_id`. |
| `language` and `published_at` on a hit | Both are `omitempty`, so with the Hurl corpus "absent" and "dropped by the driver" looked identical. One seeded document carries a language and the others do not, so both branches are asserted in one query. |
| Cropped `content` | `AttributesToRetrieve` excludes `content`; the driver reads `_formatted.content` instead. A recorded production incident had `getCropped` discarding it wholesale, giving every hit `content: ""` — which satisfies `isString`. |
| Query validation | `validateAndSanitizeQuery` rejects control characters, zero-width runes, over-long queries and queries that sanitise to nothing. None was exercised; nor was the 1000-character boundary, nor the fact that the response echoes the *sanitised* query. |
| Connect protocol conformance | Error-envelope shape and code, GET refused with 405, malformed JSON, unsupported Content-Type, and that `Connect-Protocol-Version` is not required. |
| Listener topology | Neither plaintext mux may serve the other's routes or carry the catch-all `/` that `newMTLSMuxHandler` has; `/metrics` must not appear on the unauthenticated port; nothing may listen on `:9443` while `MTLS_LISTEN=false`. |
| Access-control posture | `/v1/search` needs no credential of any kind on `:9300`, and `X-Alt-Peer-Identity` is untrusted input there. Both were true and neither was written down. |
| Meilisearch key enforcement | The suite seeds with the master key; if the key were not enforced, every isolation assertion would still pass against a world-writable index. |

### Two negatives worth calling out

`topology.spec.ts` › "nothing answers on `:9443` when MTLS_LISTEN is false" is
the one thing the Hurl suite could not express at all: Hurl treats an
unreachable server as a run failure, so the split-topology suites had to invert
the polarity outside the framework with a shell wrapper demanding exit code 3.
`expectConnectionRefused` from `_shared/net.ts` makes it an ordinary assertion —
and one that distinguishes a refused connection from a typo in the URL, which
the shell helper worked hard to preserve.

`topology.spec.ts` › "a spoofed peer-identity header changes nothing" pins
**current** behaviour, not desired behaviour. `X-Alt-Peer-Identity` is what
`PeerIdentityMiddleware` *sets* from a verified client certificate CN, after
stripping whatever the client sent; that middleware is wired only into
`newMTLSMuxHandler`, so on `:9300` the header is untrusted input nothing reads.
If a future change starts trusting it without an mTLS listener in front to
overwrite it, this test breaks — which is the intended signal.

## Parallelism and isolation

`fullyParallel: true`, four workers, no `workers: 1` project.

The ceiling is Meilisearch, not CPU and not the service's limiter. Each
worker's `corpus` fixture enqueues one `addDocuments` task and blocks until it
drains, and Meilisearch runs indexing tasks one at a time. search-indexer's own
token bucket (`SEARCH_RATE_LIMIT_RPS=100`, burst 200, `config.Load` defaults
that compose.staging.yaml does not override) is two orders of magnitude above
what four workers issuing sequential requests can produce.

The one piece of genuinely global mutable state is the driver's LRU search
cache, and no test can be served a stale entry: every query either carries a
worker-unique nonce or reads the immutable shared corpus that `globalSetup`
indexed before the first worker started.

## Out of scope

- **The rate limiter.** `middleware.NewRateLimiter` is one *global* token
  bucket, not per-caller (ADR-000717 defers caller identity), so draining it to
  observe a 429 and its `Retry-After` header would 429 every other worker for
  the duration. A `workers: 1` project would not help — Playwright projects run
  concurrently. `middleware/rate_limit_test.go` covers the bucket; what this
  suite can assert is that normal traffic is *not* limited, which every other
  test does implicitly.
- **Redis Streams ingestion.** `CONSUMER_ENABLED=false` in this slice and
  `REDIS_STREAMS_URL` points at the nginx stub. The consumer, its XAUTOCLAIM
  reclaim loop and its DLQ are covered by `app/consumer/*_test.go` and by the
  message-pact consumer tests in `app/driver/contract/`.
- **The polling index loop.** `runIndexLoop` reads articles from
  `BACKEND_API_URL`, which is the nginx stub; it retries with backoff and never
  indexes anything. Its panic-recovery behaviour is unit-tested in
  `bootstrap/index_loop_panic_test.go`.
- **Hybrid / semantic search.** `MEILI_HYBRID_EMBEDDER` is unset in staging, so
  the driver sends no `hybrid` block and `ensureEmbedderSettings` is skipped.
  Exercising it needs a live embedding Ollama instance.
- **The `:9443` mutual-TLS mux's behaviour.** Only its *absence* is asserted
  here. Turning it on would need `suite_pki`, a client leaf in
  `MTLS_ALLOWED_PEERS`, and a second set of every REST assertion behind
  `peer.Require`. The peer-identity middleware itself is unit-tested in
  `middleware/peer_identity_middleware_test.go`.
- **Meilisearch task pruning and synonym flushing.** `MEILI_TASK_PRUNE_INTERVAL`
  defaults to 6 hours and `MEILI_SYNONYMS_FLUSH_INTERVAL` to a minute with
  nothing to flush. `bootstrap/task_prune_test.go` and
  `bootstrap/synonyms_flush_test.go` cover them.
