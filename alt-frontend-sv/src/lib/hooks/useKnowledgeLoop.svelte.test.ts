import { describe, it, expect } from "vitest";
import { useKnowledgeLoop } from "./useKnowledgeLoop.svelte.ts";
import type { KnowledgeLoopResult } from "$lib/connect/knowledge_loop";

/**
 * Knowledge Loop dwell rate-limit handling.
 *
 * Production regression (2026-04-26 logs): the IntersectionObserver fires
 * a `dwell` transition (observe → orient) for every tile entering the
 * viewport. Backend §8.4 caps emissions at one per
 * (user_id, entry_key, lens_mode_id) per 60 s and returns 429 above that.
 *
 * The pre-fix `observe()` mapped the 429 response to `{status: "error"}`
 * and then `observeThrottle.reset(entryKey)` cleared the local 60 s gate.
 * The next IntersectionObserver tick re-fired immediately, hit 429 again,
 * cleared the throttle again — a tight loop the user saw as `POST .../loop/transition 429`
 * spam in the browser console.
 *
 * The contract: a 429 from the backend is a *successful* signal that the
 * window is still active. The local throttle MUST stay armed so we honor
 * the same 60 s cap on the client side.
 */

const FRESH_FOREGROUND: KnowledgeLoopResult = {
	foregroundEntries: [
		{
			entryKey: "article:42",
			sourceItemKey: "article:42",
			proposedStage: "observe",
			surfaceBucket: "now",
			projectionRevision: 1,
			projectionSeqHiwater: 100,
			freshnessAt: "2026-04-26T10:00:00Z",
			whyPrimary: { kind: "source_why", text: "fresh", evidenceRefs: [] },
			dismissState: "active",
			renderDepthHint: 4,
			loopPriority: "critical",
			decisionOptions: [],
			actTargets: [],
		},
	],
	bucketEntries: [],
	surfaces: [],
	sessionState: undefined,
	overallServiceQuality: "full",
	generatedAt: "2026-04-26T10:00:00Z",
	projectionSeqHiwater: 100,
};

function fetchReturning(status: number, body: unknown = {}): typeof fetch {
	return (async () =>
		new Response(JSON.stringify(body), {
			status,
			headers: { "content-type": "application/json" },
		})) as unknown as typeof fetch;
}

describe("useKnowledgeLoop.observe — backend 429 must NOT clear local throttle", () => {
	it("does NOT re-fire after a 429 within the 60 s window", async () => {
		let calls = 0;
		const countingFetch = (async () => {
			calls += 1;
			return new Response(JSON.stringify({ error: "rate_limited" }), {
				status: 429,
				headers: { "content-type": "application/json" },
			});
		}) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: countingFetch,
		});

		const first = await loop.observe("article:42");
		const second = await loop.observe("article:42");
		const third = await loop.observe("article:42");

		expect(first).toBe(false); // 429 → not accepted
		expect(second).toBe(false); // throttle gates the second tick
		expect(third).toBe(false); // and the third
		expect(calls).toBe(1); // backend hit ONCE, not three times
	});

	it("network error does still allow a retry (throttle reset preserved)", async () => {
		let calls = 0;
		// 500 → status "error" → reset throttle (the original behavior, preserved
		// for transient infra errors where re-tries are appropriate).
		const flakyFetch = (async () => {
			calls += 1;
			return new Response("internal", { status: 500 });
		}) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: flakyFetch,
		});

		await loop.observe("article:42");
		await loop.observe("article:42");

		expect(calls).toBe(2); // 500 keeps reset semantics — retries are OK
	});
});
