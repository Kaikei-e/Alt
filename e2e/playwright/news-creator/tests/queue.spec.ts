import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { queueStatusSchema } from "../src/schemas.js";

/**
 * `/queue/status` — the port of `02-queue-status.hurl`.
 *
 * This endpoint is a load-bearing contract, not diagnostics: pre-processor
 * backs off exponentially on `accepting == false`, and every handler in
 * news-creator answers 429 + `Retry-After: 30` once `acquire()` raises
 * `QueueFullError`. So the fields have to keep meaning what the callers think
 * they mean.
 *
 * The Hurl scenario asserted the *idle* reading — `rt_queue == 0`,
 * `be_queue == 0`, `available_slots >= 1` — which is only true of a suite
 * running one request at a time. Those exact-idle assertions moved to
 * tests/queue-drain.spec.ts, which owns a single-worker project. What is left
 * here is everything that holds no matter who else is mid-flight, and it is
 * strictly more than the Hurl version checked.
 */
test.describe("queue status", () => {
	test("GET /queue/status answers the backpressure envelope @smoke @contract", async ({
		api,
	}) => {
		const response = await api.get("/queue/status");
		const status = await expectJsonStatus(response, 200, queueStatusSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		// `max_queue_depth` is echoed straight from MAX_QUEUE_DEPTH. It is the
		// threshold every 429 in this service is derived from, so a slice that
		// silently lost the env var (the dataclass default is also 10, which is
		// exactly why an eyeball check would miss it) would move the backpressure
		// point without moving this number — hence comparing against the value
		// run.sh forwards, not a literal.
		expect(status.max_queue_depth).toBe(env.maxQueueDepth);

		// `total_slots` is OLLAMA_NUM_PARALLEL, defaulted to 1
		// (config/scheduling_config.py:46). `>= 1` rather than `== 1` because a
		// GPU deployment sets it higher and this assertion should survive that;
		// what must never happen is 0, which is what `health_handler` reports
		// when the router was handed no gateway at all.
		expect(status.total_slots).toBeGreaterThanOrEqual(1);
	});

	test("the semaphore's slot accounting balances @contract", async ({ api }) => {
		// New coverage. `queue_status()` computes `available_slots` from the two
		// pool counters and `acquired_slots` from the leak-detection map
		// (gateway/hybrid_priority_semaphore.py:846). Those are updated on
		// opposite sides of every acquire/release pair, so
		//
		//     available_slots + acquired_slots == total_slots
		//
		// is an invariant that holds at every instant regardless of load — which
		// is what makes it assertable from a parallel worker. It is also the
		// exact thing the class's own `_leak_threshold_seconds` watchdog exists
		// to catch: a handler that returns without releasing (an exception path
		// that skips `release(slot_id)`) permanently shrinks `available_slots`
		// while `acquired_slots` grows, and the service quietly degrades to
		// zero throughput. The Hurl suite could not express this — it never read
		// `acquired_slots` at all.
		const status = await expectJsonStatus(await api.get("/queue/status"), 200, queueStatusSchema);
		expect(
			status.available_slots + status.acquired_slots,
			"available_slots + acquired_slots must equal total_slots; a mismatch means " +
				"a slot was acquired and never released (or released twice)",
		).toBe(status.total_slots);
	});

	test("queue depths stay inside the configured ceiling @contract", async ({ api }) => {
		// `acquire()` refuses to enqueue past `max_queue_depth`
		// (hybrid_priority_semaphore.py:347-372), so a reading above it would
		// mean the guard was bypassed — the one state in which callers get no
		// 429 and the service accepts unbounded work.
		const status = await expectJsonStatus(await api.get("/queue/status"), 200, queueStatusSchema);
		expect(status.rt_queue + status.be_queue).toBeLessThanOrEqual(status.max_queue_depth);

		// `accepting` is `depth < max || available > 0`. Under this suite's
		// worker ceiling the depth cannot approach 10 (see playwright.config.ts
		// for the arithmetic), so a false here is a real regression in the
		// scheduler, not this test observing a busy moment.
		expect(status.accepting, `queue reported not accepting at depth ${status.rt_queue + status.be_queue}`).toBe(
			true,
		);
	});
});
