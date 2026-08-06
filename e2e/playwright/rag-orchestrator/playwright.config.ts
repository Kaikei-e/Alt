import { defineApiSuite } from "../_shared/config.js";

/**
 * rag-orchestrator API E2E.
 *
 * Two listeners, both HTTP: the Echo REST server on :9010 (health, readiness,
 * `/metrics`, the `/v1/rag/*` and `/internal/rag/*` routes) and the Connect-RPC
 * mux on :9011 (AugurService + MorningLetterService), which the staging slice
 * runs as plaintext h2c because `PEER_IDENTITY_MODE=disabled`
 * (compose/compose.staging.yaml). No spec touches `page`, `browser` or
 * `context`.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop lives
 * in `defineApiSuite` — see `_shared/config.ts`. Only the facts genuinely local
 * to rag-orchestrator are here.
 */
export default defineApiSuite({
	service: "rag-orchestrator",

	/**
	 * Sized against the **pgx pool**, which is the only bottleneck this slice
	 * has: `internal/infra/config/config.go` defaults `DB_MAX_CONNS` to 20 and
	 * `DB_MIN_CONNS` to 5, and compose.staging.yaml overrides neither. Every
	 * request this suite makes is either a single short query against rag-db
	 * (the Augur read paths) or pure in-process validation (the REST 400 cases),
	 * so six workers put at most six connections in flight — well inside 20 even
	 * with the JobWorker's 100ms `AcquireNextJob` poll (internal/worker/worker.go)
	 * and `/readyz`'s `dbPool.Ping` competing for the same pool.
	 *
	 * Nothing else throttles: rag-orchestrator has no rate limiter, and the
	 * suite deliberately never drives a path that reaches the embedder, the LLM
	 * or search-indexer — all three point at `http://127.0.0.1:9` in the slice,
	 * so a request that touched them would be a 10-30s timeout rather than a
	 * concurrency cost.
	 */
	workers: 6,

	globalSetup: "./setup/global-setup.ts",

	/**
	 * One project, no `workers: 1` companion — and that is a claim, not an
	 * omission. The two kinds of global state this service has are both
	 * unobserved by the suite:
	 *
	 *   - The JobWorker's exponential backoff (worker.go:126) is process-wide
	 *     and grows every time a seeded backfill job fails against the
	 *     unreachable embedder. `tests/rag-rest.spec.ts` asserts the *enqueue*
	 *     (HTTP 202 + a job id) and never the job's terminal status, precisely
	 *     so the shared backoff cannot make one test's result depend on
	 *     another's.
	 *   - The augur rows are seeded per *role* by `setup/db-seed.sql`, and the
	 *     only mutating test owns a conversation no other test reads. See
	 *     `src/seed.ts` for the ownership table.
	 */
});
