import { test, expect, summarizeBody } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { eventually } from "../../_shared/eventual.js";
import { queueStatusSchema, summarizeResponseSchema } from "../src/schemas.js";

/**
 * The semaphore's global counters — the half of `02-queue-status.hurl` that
 * genuinely cannot be made order-free, plus the backpressure coverage the
 * Hurl suite explicitly deferred ("Queue-saturation (HTTP 429) deferred to a
 * follow-up Phase, since reliably triggering QueueFullError from a serial
 * Hurl suite needs parallel client orchestration").
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
	test("concurrent low-priority work completes and releases every slot @slow @contract", async ({
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
		 */
		const CONCURRENCY = 3;

		const responses = await Promise.all(
			Array.from({ length: CONCURRENCY }, (_, index) =>
				api.post("/api/v1/summarize", {
					data: summarizeBody(`${seed.articleId}-${index}`, { priority: "low" }),
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
