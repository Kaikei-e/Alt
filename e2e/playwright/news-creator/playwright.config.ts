import { defineApiSuite } from "../_shared/config.js";

/**
 * news-creator API E2E — FastAPI LLM proxy on :11434.
 *
 * Drives HTTP only: the single uvicorn listener that serves the REST surface,
 * the Prometheus `/metrics` mount and the OpenAPI document. No spec touches
 * `page`, `browser` or `context`.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` — see `_shared/config.ts`. Only the facts
 * genuinely local to news-creator are here.
 */
export default defineApiSuite({
	service: "news-creator",

	/**
	 * The ceiling is the **HybridPrioritySemaphore**, not CPU.
	 *
	 * `SchedulingConfig.from_env` reads `OLLAMA_NUM_PARALLEL` with a default of
	 * 1 (news-creator/app/news_creator/config/scheduling_config.py:46) and the
	 * staging slice never sets it, so `total_slots == 1`: every request that
	 * reaches the LLM serializes behind one slot. The hard ceiling above that
	 * is `MAX_QUEUE_DEPTH=10` (compose.staging.yaml), past which `acquire()`
	 * raises `QueueFullError` and the handlers answer 429 + `Retry-After: 30`.
	 *
	 * So the number that matters is worst-case *simultaneous queue depth*.
	 * The most expensive test in the suite is the batch recap call, which fans
	 * one HTTP request out into 2 semaphore acquisitions
	 * (recap_summary_usecase.generate_batch_summary), and the drain spec,
	 * which deliberately fires 3 at once. With 4 workers the pathological
	 * arrangement is 3 (drain) + 3 other workers x 2 (batch) = 9 < 10 — under
	 * the ceiling by construction rather than by luck. Raising `workers` would
	 * turn a scheduling detail into a flaky 429.
	 *
	 * There is also exactly one uvicorn process and one event loop
	 * (Dockerfile: `uvicorn main:app --host 0.0.0.0 --port 11434`, no
	 * `--workers`), so more clients buy latency overlap, not throughput.
	 *
	 * One hazard this trades for the speedup, recorded here because it is not
	 * obvious from any spec. With `rt_reserved_slots == total_slots == 1`, a
	 * low-priority request takes the RT slot by fallback; a high-priority one
	 * arriving in that window finds `_rt_available == 0` and **preempts** it —
	 * `_has_preemptable_be()` has no minimum-runtime guard, so the only thing
	 * bounding it is how briefly a request holds the slot
	 * (hybrid_priority_semaphore.py:253-282). The staging stub answers in
	 * milliseconds, so the window is tiny, but it is not zero: an
	 * `expand-query`/`plan-query` spec (priority `high` by default) can in
	 * principle cancel a concurrent `summarize`/recap generation, which
	 * surfaces as a 500/502 rather than a wrong answer. That is exactly what
	 * `retries` + `retryStrategy: "isolated"` in `_shared/config.ts` exist for
	 * — the retry runs alone, where no other worker can preempt it. If it ever
	 * becomes noisy rather than rare, `PW_WORKERS=1` is the escape hatch.
	 */
	workers: 4,

	/**
	 * `POST /api/v1/summarize` with `stream=true` does not close its SSE body
	 * when the LLM stream ends: `stream_with_heartbeat`'s heartbeat task is
	 * parked in `await asyncio.sleep(heartbeat_interval)` with
	 * `heartbeat_interval = 10` and only then notices `stopped`, and the
	 * response loop waits for *both* sentinels
	 * (handler/summarize_handler.py). So the streaming spec takes ~10s of
	 * wall clock by design — production behaviour we do not suppress for the
	 * test. 30s left too little headroom on a loaded 2-core runner.
	 */
	timeout: 45_000,

	globalSetup: "./setup/global-setup.ts",

	projects: [
		{
			name: "api",
			testIgnore: /queue-drain\.spec\.ts$/,
		},
		/**
		 * The drain spec observes the semaphore's *global* counters — queue
		 * depths, acquired slots — which is the one thing in this suite that
		 * cannot be isolated by naming, because there is a single
		 * `HybridPrioritySemaphore` instance per process. It gets a
		 * single-worker project of its own (`TestProject.workers`) so it never
		 * races a second copy of itself.
		 *
		 * It still asserts convergence with `eventually` rather than a point
		 * reading, because Playwright's project-level `workers` bounds this
		 * project alone and the `api` project may legitimately be in flight
		 * alongside it.
		 */
		{
			name: "shared-state",
			testMatch: /queue-drain\.spec\.ts$/,
			workers: 1,
		},
	],
});
