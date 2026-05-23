import { describe, expect, it, vi } from "vitest";
import { createActOutcomeEmitter } from "./useActOutcomeEmitter.svelte";

/**
 * Unit suite for the dwell-based act-outcome emitter (ADR-000912).
 *
 * Verifies:
 *   - engaged fires once at the 30s threshold and not again on later ticks
 *   - deep_engagement fires once when dwell crosses 120s OR ask turns ≥ 3
 *   - the same (entry, outcome) tuple is not re-posted on repeated ticks
 *   - stop() removes the entry so a tick after stop posts nothing
 *   - a network-failed post leaves the outcome eligible for the next tick
 *     (server-side dedupe protects against duplicate writes once the
 *     wire actually completes)
 */
function makeRecorder() {
	const calls: Array<{
		entryKey: string;
		outcome: string;
		dwellSeconds: number;
		askTurns: number;
	}> = [];
	const post = vi.fn(
		async (args: {
			entryKey: string;
			outcome: string;
			clientOutcomeId: string;
			occurredAtIso: string;
			dwellSeconds: number;
			askTurns: number;
		}) => {
			calls.push({
				entryKey: args.entryKey,
				outcome: args.outcome,
				dwellSeconds: args.dwellSeconds,
				askTurns: args.askTurns,
			});
		},
	);
	return { calls, post };
}

describe("useActOutcomeEmitter — dwell threshold engine", () => {
	it("emits 'engaged' exactly once at the 30s threshold", async () => {
		let nowMs = 1_000_000;
		const { calls, post } = makeRecorder();
		const emitter = createActOutcomeEmitter({ post, now: () => nowMs });

		emitter.start("entry-a");

		nowMs += 29_000;
		await emitter.tick();
		expect(calls).toHaveLength(0);

		nowMs += 2_000; // now at 31s dwell
		await emitter.tick();
		expect(calls).toHaveLength(1);
		expect(calls[0]).toMatchObject({
			entryKey: "entry-a",
			outcome: "engaged",
		});
		expect(calls[0].dwellSeconds).toBeGreaterThanOrEqual(30);

		// Subsequent tick must NOT re-emit engaged.
		nowMs += 5_000;
		await emitter.tick();
		expect(calls.filter((c) => c.outcome === "engaged")).toHaveLength(1);
		emitter.teardown();
	});

	it("emits 'deep_engagement' at the 120s dwell threshold", async () => {
		let nowMs = 2_000_000;
		const { calls, post } = makeRecorder();
		const emitter = createActOutcomeEmitter({ post, now: () => nowMs });

		emitter.start("entry-b");
		nowMs += 125_000;
		await emitter.tick();

		// Both engaged AND deep_engagement fire on the same tick because dwell
		// crossed both thresholds in one step.
		const outcomes = calls.map((c) => c.outcome).sort();
		expect(outcomes).toEqual(["deep_engagement", "engaged"]);
		emitter.teardown();
	});

	it("emits 'deep_engagement' when ask turns hit 3 even before 120s dwell", async () => {
		let nowMs = 3_000_000;
		const { calls, post } = makeRecorder();
		const emitter = createActOutcomeEmitter({ post, now: () => nowMs });

		emitter.start("entry-c");
		emitter.recordAskTurn("entry-c");
		emitter.recordAskTurn("entry-c");
		emitter.recordAskTurn("entry-c");

		// Bump past engaged threshold so both outcomes are eligible; the
		// deep_engagement branch should fire on the ask-turn signal alone.
		nowMs += 35_000;
		await emitter.tick();
		const outcomes = calls.map((c) => c.outcome).sort();
		expect(outcomes).toContain("deep_engagement");
		expect(calls.find((c) => c.outcome === "deep_engagement")?.askTurns).toBe(3);
		emitter.teardown();
	});

	it("a stop() between ticks prevents any further emit for the entry", async () => {
		let nowMs = 4_000_000;
		const { calls, post } = makeRecorder();
		const emitter = createActOutcomeEmitter({ post, now: () => nowMs });

		emitter.start("entry-d");
		nowMs += 20_000;
		emitter.stop("entry-d");
		nowMs += 20_000;
		await emitter.tick();

		expect(calls).toHaveLength(0);
		emitter.teardown();
	});

	it("retries the same outcome on the next tick when post() throws", async () => {
		let nowMs = 5_000_000;
		const calls: string[] = [];
		const post = vi
			.fn()
			.mockRejectedValueOnce(new Error("network down"))
			.mockImplementation(async (args: { outcome: string }) => {
				calls.push(args.outcome);
			});
		const emitter = createActOutcomeEmitter({ post, now: () => nowMs });

		emitter.start("entry-e");
		nowMs += 31_000;
		await emitter.tick();
		expect(calls).toHaveLength(0); // first attempt failed

		nowMs += 1_000;
		await emitter.tick();
		expect(calls).toEqual(["engaged"]);
		emitter.teardown();
	});
});
