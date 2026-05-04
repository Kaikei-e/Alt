import { describe, expect, it, vi } from "vitest";
import type { KnowledgeLoopResult } from "$lib/connect/knowledge_loop";
import { useKnowledgeLoop } from "./useKnowledgeLoop.svelte.ts";

/**
 * Auto-OODA suppression (Knowledge Loop 体験回復プラン Pillar 1).
 *
 * The dwell-fired `(observe, orient, dwell)` transition is removed entirely.
 * Passive viewing must NOT advance OODA stage — Boyd's OODA model treats
 * Orientation as a conscious step, and Linear-class command-center UIs
 * separate read-state from workflow status.
 *
 * The hook no longer exposes an `observe()` method. The only way to advance
 * out of Observe is an explicit `transitionTo(entryKey, "orient", "user_tap")`
 * driven by a user gesture (tap-to-expand on the tile).
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

const REVIEW_BUCKET: KnowledgeLoopResult = {
	...FRESH_FOREGROUND,
	foregroundEntries: [],
	bucketEntries: [
		{
			...FRESH_FOREGROUND.foregroundEntries[0],
			entryKey: "article:review-1",
			sourceItemKey: "article:review-1",
			surfaceBucket: "review",
			proposedStage: "observe",
		},
	],
};

describe("useKnowledgeLoop — dwell removed, no observe() method", () => {
	it("does not expose observe() any more (passive stage advancement is suppressed)", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: (async () =>
				new Response("{}", { status: 200 })) as unknown as typeof fetch,
		});
		// The only legitimate way to leave Observe is now an explicit user_tap
		// transition. The hook surface MUST NOT carry a method that fires
		// (observe, orient, dwell) implicitly.
		expect("observe" in loop).toBe(false);
	});

	it("never fires a /loop/transition POST without an explicit caller invoking transitionTo", async () => {
		let calls = 0;
		const countingFetch = (async () => {
			calls += 1;
			return new Response(
				JSON.stringify({ accepted: true, canonicalEntryKey: "article:42" }),
				{ status: 200 },
			);
		}) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: countingFetch,
		});
		await Promise.resolve();
		await Promise.resolve();
		expect(calls).toBe(0);

		const result = await loop.transitionTo("article:42", "orient", "user_tap");
		expect(result.status).toBe("accepted");
		expect(calls).toBe(1);
	});
});

describe("useKnowledgeLoop.bucketEntries — review lane state ownership", () => {
	it("exposes bucket entries from the initial snapshot", () => {
		const loop = useKnowledgeLoop({
			initial: REVIEW_BUCKET,
			lensModeId: "default",
			fetchImpl: (async () =>
				new Response("{}", { status: 200 })) as unknown as typeof fetch,
		});

		expect(loop.entries).toHaveLength(0);
		expect(loop.bucketEntries).toHaveLength(1);
		expect(loop.bucketEntries[0].entryKey).toBe("article:review-1");
	});

	it("posts Review actions for bucket entries and optimistically removes completed actions", async () => {
		const calls: Array<Record<string, unknown>> = [];
		const captureFetch = (async (_url: unknown, init?: { body?: string }) => {
			calls.push(JSON.parse(init?.body ?? "{}"));
			return new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			});
		}) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: REVIEW_BUCKET,
			lensModeId: "default",
			fetchImpl: captureFetch,
		});

		const result = await loop.reviewAction("article:review-1", "archive");

		expect(result.status).toBe("accepted");
		expect(calls).toHaveLength(1);
		expect(calls[0].trigger).toBe("archive");
		expect(calls[0].fromStage).toBe("observe");
		expect(calls[0].toStage).toBe("observe");
		expect(loop.bucketEntries).toHaveLength(0);
	});

	it("filters optimistically dismissed bucket entries during snapshot replacement", async () => {
		const accepted = (async () =>
			new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			})) as unknown as typeof fetch;
		const loop = useKnowledgeLoop({
			initial: REVIEW_BUCKET,
			lensModeId: "default",
			fetchImpl: accepted,
		});

		await loop.reviewAction("article:review-1", "mark_reviewed");
		loop.replaceSnapshot(REVIEW_BUCKET);

		expect(loop.bucketEntries).toHaveLength(0);
	});

	it("applies stream inline entries without a coalesced refetch", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: (async () =>
				new Response("{}", { status: 200 })) as unknown as typeof fetch,
		});

		const applied = loop.applyStreamFrame({
			kind: "appended",
			entryKey: "article:continue-1",
			revision: 101n,
			projectionSeqHiwater: 101n,
			inlineEntry: {
				...FRESH_FOREGROUND.foregroundEntries[0],
				entryKey: "article:continue-1",
				sourceItemKey: "article:continue-1",
				surfaceBucket: "continue",
				projectionRevision: 2,
				projectionSeqHiwater: 101,
			},
		});

		expect(applied).toBe(true);
		expect(loop.entries.map((e) => e.entryKey)).toEqual(["article:42"]);
		expect(loop.bucketEntries.map((e) => e.entryKey)).toEqual([
			"article:continue-1",
		]);
	});
});

describe("useKnowledgeLoop.currentEntryStage — proposed stage is not local progress", () => {
	it("uses currentEntryStage as transition fromStage without mutating proposedStage", async () => {
		const calls: Array<Record<string, unknown>> = [];
		const captureFetch = (async (_url: unknown, init?: { body?: string }) => {
			calls.push(JSON.parse(init?.body ?? "{}"));
			return new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			});
		}) as unknown as typeof fetch;
		const loop = useKnowledgeLoop({
			initial: {
				...FRESH_FOREGROUND,
				foregroundEntries: [
					{
						...FRESH_FOREGROUND.foregroundEntries[0],
						proposedStage: "observe",
						currentEntryStage: "orient",
					},
				],
			},
			lensModeId: "default",
			fetchImpl: captureFetch,
		});

		const result = await loop.transitionTo("article:42", "decide");

		expect(result.status).toBe("accepted");
		expect(calls[0].fromStage).toBe("orient");
		expect(calls[0].toStage).toBe("decide");
		expect(loop.entries[0].proposedStage).toBe("observe");
		expect(loop.entries[0].currentEntryStage).toBe("decide");
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
		});

		await loop.dismiss("does-not-exist");
		expect(loop.entries).toHaveLength(1);
	});

	// Persistence regression (canonical contract §8.2): dismiss must always
	// route to KnowledgeLoopDeferred — same-stage transition with the `defer`
	// trigger — regardless of the entry's proposedStage. Pre-fix the hook
	// short-circuited on observe/orient/act because it tried to force a
	// `decide → act` transition that those stages can't make, so the server
	// was never told and the projection still considered the entry active.
	it("posts a same-stage DEFER transition on dismiss (observe entry)", async () => {
		const calls: Array<Record<string, unknown>> = [];
		const captureFetch = (async (_url: unknown, init?: { body?: string }) => {
			calls.push(JSON.parse(init?.body ?? "{}"));
			return new Response(JSON.stringify({ accepted: true }), {
				status: 200,
				headers: { "content-type": "application/json" },
			});
		}) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND, // proposedStage === "observe"
			lensModeId: "default",
			fetchImpl: captureFetch,
		});

		await loop.dismiss("article:42");

		expect(calls).toHaveLength(1);
		const body = calls[0];
		expect(body.trigger).toBe("defer");
		expect(body.fromStage).toBe("observe");
		expect(body.toStage).toBe("observe");
		expect(body.entryKey).toBe("article:42");
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

/**
 * Production regression (re-occurring): when the user backgrounds the tab for
 * a long time and an in-flight `/loop/transition` request never resolves
 * (server JWT expiry mid-flight, network drop, bfcache freeze), the hook's
 * `try/finally { inFlight.delete(...) }` never runs. A subsequent
 * `replaceSnapshot()` (driven by `invalidate("loop:data")`) re-seeds entries
 * but does not garbage-collect stale `inFlight` keys, so `isInFlight(key)`
 * keeps returning true forever and `LoopEntryTile`'s `disabled={inFlight}`
 * gate locks the buttons.
 *
 * Three-layer defense:
 *   (a) Per-call AbortController + 8s timeout so the `finally` always fires.
 *   (b) `replaceSnapshot` gc: drop inFlight keys absent from the next snapshot
 *       OR whose start timestamp is older than the timeout window.
 *   (c) `resetInFlight(reason)` for the visibility-change recovery path.
 */
describe("useKnowledgeLoop.replaceSnapshot — inFlight stale gc", () => {
	it("clears inFlight keys not present in the next snapshot", async () => {
		// Stall the first fetch so `inFlight` retains the key, then call
		// replaceSnapshot with an entirely different foreground.
		let resolveFirst!: (r: Response) => void;
		const stallingFetch = ((..._args: unknown[]) =>
			new Promise<Response>((res) => {
				resolveFirst = res;
			})) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: stallingFetch,
		});

		// Kick off a transition that will hang.
		const pending = loop.transitionTo("article:42", "orient");
		// Yield once so the synchronous `inFlight.add(...)` runs before our
		// assertion below.
		await Promise.resolve();
		expect(loop.isInFlight("article:42")).toBe(true);

		// New snapshot does NOT include article:42 anymore.
		loop.replaceSnapshot({
			...FRESH_FOREGROUND,
			foregroundEntries: [
				{
					...FRESH_FOREGROUND.foregroundEntries[0],
					entryKey: "article:99",
					projectionRevision: 5,
				},
			],
			projectionSeqHiwater: 500,
		});

		expect(loop.isInFlight("article:42")).toBe(false);

		// Cleanup — let the stalled fetch resolve so the test does not leak.
		resolveFirst(
			new Response(JSON.stringify({ accepted: true }), { status: 200 }),
		);
		await pending;
	});

	it("retains inFlight for keys still present in the next snapshot and started recently", async () => {
		let resolveFirst!: (r: Response) => void;
		const stallingFetch = ((..._args: unknown[]) =>
			new Promise<Response>((res) => {
				resolveFirst = res;
			})) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: stallingFetch,
		});

		const pending = loop.transitionTo("article:42", "orient");
		await Promise.resolve();
		expect(loop.isInFlight("article:42")).toBe(true);

		// Same foreground (article:42 still present), recent start → keep gating.
		loop.replaceSnapshot(FRESH_FOREGROUND);
		expect(loop.isInFlight("article:42")).toBe(true);

		resolveFirst(
			new Response(JSON.stringify({ accepted: true }), { status: 200 }),
		);
		await pending;
	});
});

describe("useKnowledgeLoop — per-call AbortController timeout", () => {
	it("aborts a stalled transitionTo after the timeout and clears inFlight", async () => {
		// Capture the AbortSignal so we can inspect that the hook actually
		// installs one. Using a real timer would make this test slow; instead
		// we verify the abort path by listening for the 'abort' event the hook
		// must trigger when the deadline expires.
		let installedSignal: AbortSignal | undefined;
		const stallingFetch = ((_url: unknown, init?: RequestInit) => {
			installedSignal = init?.signal as AbortSignal | undefined;
			return new Promise<Response>((_res, rej) => {
				init?.signal?.addEventListener("abort", () => {
					rej(new DOMException("aborted", "AbortError"));
				});
			});
		}) as unknown as typeof fetch;

		vi.useFakeTimers();
		try {
			const loop = useKnowledgeLoop({
				initial: FRESH_FOREGROUND,
				lensModeId: "default",
				fetchImpl: stallingFetch,
			});

			const pending = loop.transitionTo("article:42", "orient");
			// allow the hook to install its signal + timeout
			await Promise.resolve();
			expect(installedSignal).toBeDefined();
			expect(loop.isInFlight("article:42")).toBe(true);

			// Advance past the 8s timeout.
			await vi.advanceTimersByTimeAsync(9_000);
			const result = await pending;

			expect(result.status).toBe("error");
			expect(loop.isInFlight("article:42")).toBe(false);
			expect(installedSignal?.aborted).toBe(true);
		} finally {
			vi.useRealTimers();
		}
	});
});

describe("useKnowledgeLoop.resetInFlight", () => {
	it("clears every tracked inFlight key", async () => {
		// One resolver per call so the second `transitionTo` doesn't overwrite
		// the first resolver and leak a never-settling promise.
		const resolvers: Array<(r: Response) => void> = [];
		const stallingFetch = ((..._args: unknown[]) =>
			new Promise<Response>((res) => {
				resolvers.push(res);
			})) as unknown as typeof fetch;

		const loop = useKnowledgeLoop({
			initial: {
				...FRESH_FOREGROUND,
				foregroundEntries: [
					FRESH_FOREGROUND.foregroundEntries[0],
					{
						...FRESH_FOREGROUND.foregroundEntries[0],
						entryKey: "article:43",
					},
				],
			},
			lensModeId: "default",
			fetchImpl: stallingFetch,
		});

		const a = loop.transitionTo("article:42", "orient");
		const b = loop.transitionTo("article:43", "orient");
		// Yield twice so both microtasks (and therefore both `inFlight.add`
		// calls) have run before we assert.
		await Promise.resolve();
		await Promise.resolve();
		expect(loop.isInFlight("article:42")).toBe(true);
		expect(loop.isInFlight("article:43")).toBe(true);

		loop.resetInFlight("visibility");

		expect(loop.isInFlight("article:42")).toBe(false);
		expect(loop.isInFlight("article:43")).toBe(false);

		// Drain — fulfil both pending fetches so the test does not leak
		// promises into the next test.
		for (const r of resolvers) {
			r(new Response(JSON.stringify({ accepted: true }), { status: 200 }));
		}
		await Promise.allSettled([a, b]);
	});
});

const NOOP_FETCH = (async () =>
	new Response("{}", { status: 200 })) as unknown as typeof fetch;

const BUCKET_ENTRY_CONTINUE: KnowledgeLoopResult["bucketEntries"][number] = {
	...FRESH_FOREGROUND.foregroundEntries[0],
	entryKey: "article:continue-99",
	sourceItemKey: "article:continue-99",
	surfaceBucket: "continue",
	projectionRevision: 3,
	projectionSeqHiwater: 200,
};

describe("useKnowledgeLoop.applyStreamFrame — inline patch protocol", () => {
	it("revised with inlineEntry patches the matching foreground entry in-place", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const revised = {
			...FRESH_FOREGROUND.foregroundEntries[0],
			projectionRevision: 7,
		};
		const applied = loop.applyStreamFrame({
			kind: "revised",
			entryKey: "article:42",
			revision: 102n,
			projectionSeqHiwater: 102n,
			inlineEntry: revised,
		});

		expect(applied).toBe(true);
		expect(loop.entries).toHaveLength(1);
		expect(loop.entries[0].projectionRevision).toBe(7);
	});

	it("withdrawn removes the entry from foreground and returns true", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const applied = loop.applyStreamFrame({
			kind: "withdrawn",
			entryKey: "article:42",
			revision: 103n,
			projectionSeqHiwater: 103n,
		});

		expect(applied).toBe(true);
		expect(loop.entries).toHaveLength(0);
	});

	it("withdrawn removes a bucket entry and returns true", () => {
		const loop = useKnowledgeLoop({
			initial: { ...FRESH_FOREGROUND, bucketEntries: [BUCKET_ENTRY_CONTINUE] },
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const applied = loop.applyStreamFrame({
			kind: "withdrawn",
			entryKey: "article:continue-99",
			revision: 104n,
			projectionSeqHiwater: 104n,
		});

		expect(applied).toBe(true);
		expect(loop.bucketEntries).toHaveLength(0);
	});

	it("appended without inlineEntry returns false (signals fallback to invalidate)", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const applied = loop.applyStreamFrame({
			kind: "appended",
			entryKey: "article:new-1",
			revision: 105n,
			projectionSeqHiwater: 105n,
			// inlineEntry intentionally absent
		});

		expect(applied).toBe(false);
		expect(loop.entries).toHaveLength(1); // unchanged
	});

	it("superseded removes old entryKey from foreground and returns true", () => {
		const loop = useKnowledgeLoop({
			initial: FRESH_FOREGROUND,
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const applied = loop.applyStreamFrame({
			kind: "superseded",
			entryKey: "article:42",
			newEntryKey: "article:42-v2",
			revision: 106n,
			projectionSeqHiwater: 106n,
		});

		expect(applied).toBe(true);
		expect(
			loop.entries.find((e) => e.entryKey === "article:42"),
		).toBeUndefined();
	});

	it("superseded removes old entryKey from bucketEntries and returns true", () => {
		const loop = useKnowledgeLoop({
			initial: { ...FRESH_FOREGROUND, bucketEntries: [BUCKET_ENTRY_CONTINUE] },
			lensModeId: "default",
			fetchImpl: NOOP_FETCH,
		});

		const applied = loop.applyStreamFrame({
			kind: "superseded",
			entryKey: "article:continue-99",
			newEntryKey: "article:continue-99-v2",
			revision: 107n,
			projectionSeqHiwater: 107n,
		});

		expect(applied).toBe(true);
		expect(
			loop.bucketEntries.find((e) => e.entryKey === "article:continue-99"),
		).toBeUndefined();
	});
});
