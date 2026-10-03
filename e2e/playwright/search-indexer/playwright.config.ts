import { defineApiSuite } from "../_shared/config.js";

/**
 * search-indexer API E2E.
 *
 * Listeners in scope:
 *
 *   :9300  plaintext health only — `/health` + `/health/deep` (`bootstrap/servers.go`,
 *          `newHTTPServer`)
 *   :9301  retired plaintext Connect-RPC listener (asserted closed/refused)
 *   :9443  mutual-TLS mux serving REST `/v1/search` and Connect-RPC
 *          `services.search.v2.SearchService` with peer-identity gating
 *
 * The retired Hurl suite covered only :9300 and listed the other two under
 * "out of scope" — Connect because h2c is impractical from Hurl, mTLS because
 * a refused connection is a run failure there rather than an assertion.
 * Neither limitation applies to Playwright: `APIRequestContext` speaks
 * HTTP/1.1 (which connect-go serves for unary JSON calls just as happily as
 * h2c) and rejects on a transport error, which `_shared/net.ts` turns into a
 * normal assertion.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` — see `_shared/config.ts`.
 */
export default defineApiSuite({
	service: "search-indexer",

	/**
	 * Sized against Meilisearch, not CPU, and not against the service's own
	 * limiter.
	 *
	 * The bottleneck downstream of every test here is the single
	 * `meilisearch` container. Each worker's `corpus` fixture enqueues one
	 * `addDocuments` task and blocks until Meilisearch reports it `succeeded`;
	 * Meilisearch runs its indexing tasks one at a time, so worker N+1's seed
	 * does not start until worker N's has drained. Four workers overlap the
	 * read-only specs (which are the overwhelming majority) while keeping the
	 * seed queue to a length a cold container clears in a couple of seconds.
	 *
	 * search-indexer's own rate limiter is not the constraint: `config.Load`
	 * defaults `SEARCH_RATE_LIMIT_RPS=100` / `SEARCH_RATE_LIMIT_BURST=200` and
	 * compose.staging.yaml overrides neither, which is two orders of magnitude
	 * above what four workers issuing sequential requests can produce.
	 *
	 * No `workers: 1` project is needed, and that is a property of the design
	 * rather than luck. The one piece of genuinely global mutable state in
	 * this service is the driver's LRU search cache
	 * (`MEILI_SEARCH_CACHE_SIZE=1024`, 5-minute TTL, `driver/meilisearch_cache.go`),
	 * whose key is `{query, user_id, filter, offset, limit}` — and *negative*
	 * results are cached too. Every query in this suite therefore either
	 * carries a worker-unique nonce, or reads the immutable shared corpus that
	 * `setup/global-setup.ts` has already indexed before the first worker
	 * starts. No test can ever be served a cached miss for data that landed
	 * after the miss was recorded.
	 */
	workers: 4,

	globalSetup: "./setup/global-setup.ts",
});
