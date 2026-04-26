import { describe, expect, it } from "vitest";
import type { KnowledgeLoopResult } from "$lib/connect/knowledge_loop";
import { useKnowledgeLoop } from "./useKnowledgeLoop.svelte.ts";

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
			observeThrottleStorage: null,
		});

		const first = await loop.observe("article:42");
		const second = await loop.observe("article:42");
		const third = await loop.observe("article:42");

		expect(first).toBe(false); // 429 → not accepted
		expect(second).toBe(false); // throttle gates the second tick
		expect(third).toBe(false); // and the third
		expect(calls).toBe(1); // backend hit ONCE, not three times
	});

	it("persists throttle to injected Storage so a fresh hook (reload) does not re-fire", async () => {
		// First "page session": single in-memory observe → 429 captured →
		// throttle persisted to the shared Storage.
		const storage = (() => {
			const m = new Map<string, string>();
			return {
				getItem: (k: string) => m.get(k) ?? null,
				setItem: (k: string, v: string) => {
					m.set(k, v);
				},
				removeItem: (k: string) => {
					m.delete(k);
				},
			};
		})();
		let calls = 0;
		const fetchImpl = (async () => {
			calls += 1;
			return new Response(JSON.stringify({ error: "rate_limited" }), {
				status: 429,
			});
		}) as unknown as typeof fetch;

		const session1 = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl,
			observeThrottleStorage: storage,
		});
		await session1.observe("article:42");
		expect(calls).toBe(1);

		// Second "page session" reuses the same Storage — exactly what a
		// browser reload looks like. The throttle MUST gate the new hook so
		// no second fetch is issued, mirroring backend §8.4's 60s window.
		const session2 = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl,
			observeThrottleStorage: storage,
		});
		await session2.observe("article:42");
		await session2.observe("article:42");
		expect(calls).toBe(1);
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
			observeThrottleStorage: null,
		});

		await loop.observe("article:42");
		await loop.observe("article:42");

		expect(calls).toBe(2); // 500 keeps reset semantics — retries are OK
	});
});

/**
 * Dismiss must remove the entry from `entries`, not just flag it as
 * `dismissed`. The pre-fix behaviour kept the entry in the keyed `#each`
 * with a `.dismissing` class that collapsed `max-height` — combined with the
 * fetch storm starving the main thread, the half-collapsed tile bled into
 * its neighbors. The new contract: dismiss removes the row immediately so
 * Svelte's `out:` transition + `animate:flip` can play on the parent.
 */
describe("useKnowledgeLoop.dismiss — removes the entry from foreground", () => {
	it("removes the dismissed entry from `entries` after the local apply", async () => {
		const accepted = (async () =>
			new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			})) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: accepted,
			observeThrottleStorage: null,
		});
		expect(loop.entries).toHaveLength(1);

		await loop.dismiss("article:42");
		expect(loop.entries).toHaveLength(0);
	});

	it("dismissing an unknown entry is a no-op", async () => {
		const fetchImpl = (async () =>
			new Response("{}", { status: 200 })) as unknown as typeof fetch;
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl,
			observeThrottleStorage: null,
		});

		await loop.dismiss("does-not-exist");
		expect(loop.entries).toHaveLength(1);
	});
});

/**
 * `replaceSnapshot` re-seeds entries and sessionState from a freshly fetched
 * GetKnowledgeLoop result without forcing an SSR `__data.json` refetch. The
 * coalesced stream-driven refresh feeds it. Optimistically dismissed entries
 * (those the user just removed locally) must NOT come back, even if the
 * server-side projection has not caught up to the dismiss event yet.
 */
describe("useKnowledgeLoop.replaceSnapshot — re-seed without losing optimistic state", () => {
	it("replaces entries when no optimistic dismissals are pending", async () => {
		const fetchImpl = (async () =>
			new Response("{}", { status: 200 })) as unknown as typeof fetch;
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl,
			observeThrottleStorage: null,
		});

		const next: KnowledgeLoopResult = {
			...FRESH_FOREGROUND,
			foregroundEntries: [
				{
					...FRESH_FOREGROUND.foregroundEntries[0],
					entryKey: "article:99",
					projectionRevision: 2,
				},
			],
			projectionSeqHiwater: 200,
		};

		loop.replaceSnapshot(next);
		expect(loop.entries).toHaveLength(1);
		expect(loop.entries[0].entryKey).toBe("article:99");
	});

	it("does NOT resurrect an entry the user just dismissed locally", async () => {
		const accepted = (async () =>
			new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			})) as unknown as typeof fetch;
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: accepted,
			observeThrottleStorage: null,
		});

		await loop.dismiss("article:42");
		expect(loop.entries).toHaveLength(0);

		// Server-side projection has not caught up yet — it still returns the
		// dismissed entry. The hook must filter it out so the UI does not flash
		// the re-appearance of a tile the user just removed.
		loop.replaceSnapshot(FRESH_FOREGROUND);
		expect(
			loop.entries.find((e) => e.entryKey === "article:42"),
		).toBeUndefined();
	});
});
