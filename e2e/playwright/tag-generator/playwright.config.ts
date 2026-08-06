import { defineApiSuite } from "../_shared/config.js";

/**
 * tag-generator API E2E.
 *
 * Two surfaces, both HTTP:
 *   - FastAPI on :9400 — `/health`, `/api/v1/extract-tags`, and the two
 *     authenticated routes that fail closed in this slice.
 *   - mq-hub's Connect-RPC on :9500 — the only way to drive tag-generator's
 *     Redis Streams consumer from a test, since nothing here speaks XREAD.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` (`_shared/config.ts`). Only the two facts that are
 * genuinely tag-generator's own are here.
 */
export default defineApiSuite({
	service: "tag-generator",

	/**
	 * The bottleneck is the service under test itself, not CPU and not a DB
	 * pool: tag-generator is **one Python process**.
	 *
	 *   - `Dockerfile.tag-generator` runs `python auth_service.py`, i.e. a
	 *     single uvicorn worker, and pins `OMP_NUM_THREADS=1` /
	 *     `MKL_NUM_THREADS=1`, so the SBERT forward pass gets one BLAS thread.
	 *   - `/api/v1/extract-tags` runs the extractor through
	 *     `asyncio.to_thread` (auth_service.py:559), so N concurrent callers
	 *     contend for one GIL and one model instance.
	 *   - The stream path is narrower still: `_run_background_tag_generation`
	 *     starts exactly one `redis-streams-tags-consumer` thread
	 *     (auth_service.py:383-388), which handles `alt:events:tags` strictly
	 *     serially.
	 *
	 * So the useful worker count is small. 2 lets the cheap validation and
	 * routing tests overlap the ML ones — which is most of the wall-clock win
	 * — without stacking three simultaneous inferences behind one GIL and
	 * pushing an already-slow test past its budget. Raising this does not make
	 * the suite faster; it makes each ML test slower by the same factor.
	 */
	workers: 2,

	/**
	 * 60s rather than the fleet default of 30s. The Redis Streams round trip
	 * alone asks mq-hub to block for up to 30s waiting on a reply
	 * (generate_tags_usecase.go clamps `timeout_ms`), and a cold-ish KeyBERT
	 * pass on a 2-core CI runner sits on top of that. The readiness gate has
	 * already warmed the model before any test runs, so this is headroom, not
	 * an expectation.
	 */
	timeout: 60_000,

	globalSetup: "./setup/global-setup.ts",
});
