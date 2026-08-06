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
 * here is everything that holds no matter who else is mid-flight.
 *
 * **Known gap: the 429 path is not gated by this suite.** Two assertions that
 * used to live here — `rt_queue + be_queue <= max_queue_depth` and
 * `accepting == true` — were removed rather than kept, because neither could
 * fail. `acquire()` raises `QueueFullError` *before* the `heapq.heappush`
 * whenever `current_depth >= self._max_queue_depth`
 * (hybrid_priority_semaphore.py:347-372), so a depth above the ceiling is
 * unreachable through the public API; and `accepting` is
 * `depth < max || available > 0`, whose first disjunct always holds while this
 * suite's arithmetic caps worst-case depth at 9 against a ceiling of 10.
 *
 * Closing the gap needs a differently-configured slice, not another assertion.
 * Driving depth past `MAX_QUEUE_DEPTH=10` from here would mean saturating the
 * one process-global semaphore, which the concurrently-running `api` project
 * shares — every sibling test in flight would start collecting the 429s this
 * spec was fishing for. The 429 + `Retry-After: 30` route
 * (generate_handler.py:112-121, summarize_handler.py:349-359,
 * chat_handler.py:137-143) wants its own compose slice with `MAX_QUEUE_DEPTH=1`
 * and no sibling project. Until then it is covered by unit tests only.
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

	test("no slot is leaked or double-counted @contract", async ({ api }) => {
		// New coverage. `queue_status()` computes `available_slots` from the two
		// pool counters and `acquired_slots` from the leak-detection map
		// (gateway/hybrid_priority_semaphore.py:846). Those are updated on
		// opposite sides of every acquire/release pair, so their sum tracks
		// `total_slots` — which is the exact thing the class's own
		// `_leak_threshold_seconds` watchdog exists to catch: a handler that
		// returns without releasing (an exception path that skips
		// `release(slot_id)`) permanently shrinks `available_slots` while
		// `acquired_slots` grows, and the service quietly degrades to zero
		// throughput. The Hurl suite could not express this — it never read
		// `acquired_slots` at all.
		//
		// The band, not equality. `release()`'s own invariant check documents
		// why: "When woke_up is True, a slot is 'in transit' — removed from
		// `_acquired_slots` by `_track_release()` but not yet tracked by
		// `_track_acquire()` in the acquiring coroutine. This is normal", and it
		// compensates with `expected_total -= 1`
		// (hybrid_priority_semaphore.py:630-643). That window spans an await
		// boundary — `release()` hands the slot over with
		// `future.set_result(...)` while the waiter only calls `_track_acquire`
		// after its `await future` resumes — and `/queue/status` is an ordinary
		// coroutine on the same event loop, so it can be serviced inside the
		// gap. With `total_slots == 1` and four workers queued behind it, that
		// handoff is continuous, so pinning equality here would fail
		// intermittently on a live stack for no defect.
		//
		// `total_slots - 1` is therefore legal; `total_slots - 2` is not, and
		// neither is anything above `total_slots`. The strict `== total_slots`
		// form still exists where it is sound — inside the `eventually` block of
		// queue-drain.spec.ts, which owns the single-worker project and asserts
		// at rest.
		const status = await expectJsonStatus(await api.get("/queue/status"), 200, queueStatusSchema);
		const tracked = status.available_slots + status.acquired_slots;
		expect(
			tracked,
			"available_slots + acquired_slots fell more than one below total_slots — at " +
				"most one slot may be in transit between a release and the waiter it woke; " +
				"a larger shortfall means a slot was acquired and never released",
		).toBeGreaterThanOrEqual(status.total_slots - 1);
		expect(
			tracked,
			"available_slots + acquired_slots exceeded total_slots — a slot was released " +
				"twice, or returned to a pool it never came from",
		).toBeLessThanOrEqual(status.total_slots);
	});
});
