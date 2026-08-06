import { test, expect, summarizeBody } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { eventually } from "../../_shared/eventual.js";
import { queueStatusSchema, summarizeResponseSchema } from "../src/schemas.js";

/**
 * The semaphore's global counters — the half of `02-queue-status.hurl` that
 * genuinely cannot be made order-free.
 *
 * This is the parallel-client orchestration the Hurl suite could not do, but
 * it is *not* the queue-saturation coverage that suite deferred: three
 * concurrent requests against `MAX_QUEUE_DEPTH=10` never reach
 * `QueueFullError`, so the 429 + `Retry-After: 30` path is still ungated. See
 * the header of tests/queue.spec.ts for why closing that needs its own
 * compose slice rather than a bigger number here.
 *
 * There is exactly one `HybridPrioritySemaphore` per uvicorn process, so
 * `rt_queue` / `be_queue` / `acquired_slots` are process-global state that no
 * amount of per-test naming can isolate. This file therefore runs in the
 * `shared-state` project (`workers: 1`, see playwright.config.ts) so it never
 * races a second copy of itself.
 *
 * It still converges with `eventually` instead of taking a point reading:
 * Playwright's project-level `workers` bounds this project alone, and the
 * `api` project may legitimately have a request in flight alongside it. The
 * claim being asserted is "the queue drains", which is a statement about
 * time, not about a single sample — and asserting it with a sleep would be
 * the same test with a worse failure message.
 */
test.describe("semaphore drains back to idle", () => {
	test("concurrent work completes and releases every slot @slow @contract", async ({
		api,
		seed,
	}) => {
		/**
		 * Three at once, deliberately.
		 *
		 * `total_slots` is 1 in the staging slice, so these serialize behind one
		 * slot and two of them must *queue* — which is the state the Hurl suite
		 * could never reach, and the only way to exercise `acquire()`'s enqueue
		 * path end-to-end. Three also keeps worst-case depth under
		 * MAX_QUEUE_DEPTH=10 even with the `api` project's batch spec running
		 * (arithmetic in playwright.config.ts), so a 429 here is a real
		 * regression rather than this test tripping its own ceiling.
		 *
		 * `priority: "high"`, and that is load-bearing rather than incidental.
		 * These were low-priority, which made them **preemptable by a sibling
		 * project**: `workers: 1` bounds this project alone and neither project
		 * declares `dependencies`, so the `api` project runs alongside it, and
		 * `chat_generate` defaults to `priority: "high"`
		 * (ollama_gateway.py:969) — chat, the streaming specs and plan-query all
		 * acquire high. With `SCHEDULING_RT_RESERVED_SLOTS=1` and one total slot
		 * `be_slots == 0`, so a low-priority request holds the single RT slot by
		 * fallback (hybrid_priority_semaphore.py:421-429) — precisely the state
		 * where an arriving RT caller finds `_rt_available == 0` and runs
		 * `_preempt_oldest_be()`, which has no minimum-runtime guard
		 * (hybrid_priority_semaphore.py:253-282, 400-405). The victim is
		 * cancelled mid-generation and surfaces as a non-200, which the loop
		 * below would report as a queueing regression it is not. High-priority
		 * requests are never preemption candidates (`_has_preemptable_be` looks
		 * only at `not req.is_high_priority`), and they still serialize and
		 * still queue, so the enqueue path this spec exists for is unchanged.
		 */
		const CONCURRENCY = 3;

		const responses = await Promise.all(
			Array.from({ length: CONCURRENCY }, (_, index) =>
				api.post("/api/v1/summarize", {
					data: summarizeBody(`${seed.articleId}-${index}`, { priority: "high" }),
				}),
			),
		);

		for (const [index, response] of responses.entries()) {
			// Every one must be a 200, not a 429: at depth 2 the queue is nowhere
			// near its ceiling, so a QueueFullError here would mean `acquire()`
			// is rejecting work it has room for. And each must echo *its own*
			// article_id — the proof that the queue is a queue and not a place
			// where concurrent requests get each other's answers.
			const body = await expectJsonStatus(response, 200, summarizeResponseSchema);
			expect(body.article_id).toBe(`${seed.articleId}-${index}`);
		}

		await eventually(
			async () => {
				const status = await expectJsonStatus(
					await api.get("/queue/status"),
					200,
					queueStatusSchema,
				);
				expect(status.rt_queue).toBe(0);
				expect(status.be_queue).toBe(0);
				// The strong form: not merely "nothing waiting" but "nothing held".
				// A handler that returned its response without releasing its slot
				// would leave `acquired_slots` at 1 forever — the failure mode the
				// class's leak watchdog is built for, and one that is invisible to
				// a queue-depth-only check because a leaked slot is not a queued
				// request.
				expect(status.acquired_slots).toBe(0);
				expect(status.available_slots).toBe(status.total_slots);
				expect(status.accepting).toBe(true);
			},
			{
				timeout: 40_000,
				message:
					"HybridPrioritySemaphore returns to fully idle (no queued requests, " +
					"no held slots) after the concurrent summarize batch completes",
			},
		);
	});
});
