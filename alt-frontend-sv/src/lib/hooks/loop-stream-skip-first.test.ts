import { describe, expect, it, vi } from "vitest";
import { makeFirstFrameSkipper } from "./loop-stream-skip-first";

/**
 * The "skip first non-silent frame" guard for the Knowledge Loop stream.
 *
 * Production symptom (2026-04-26): the page mounts with SSR-loaded
 * `data.loop`, the stream subscription opens in `onMount`, and the *very
 * first* non-silent frame the server sends restates state the SSR HTML
 * already inlined. The page-level `onFrame` callback used to call
 * `coalescedRefresh.trigger()` for that frame too, kicking an immediate
 * `invalidate("loop:data")` mid-hydration. SvelteKit's `data` prop is
 * replaced (not the component recreated, per kit/load docs), but a
 * keyed `{#each}` over a fresh `entries` array reference re-runs its
 * keying pass and any in-flight click on a child article is dropped.
 *
 * The guard skips the first non-silent frame because the SSR snapshot
 * is already authoritative for that state; subsequent frames are real
 * deltas and must trigger the coalesced refresh as usual.
 *
 * The helper is pure (no Svelte runes) so it is unit-testable here.
 */

describe("makeFirstFrameSkipper", () => {
	it("swallows the first call (the server-inlined initial frame)", () => {
		const trigger = vi.fn();
		const onFrame = makeFirstFrameSkipper(trigger);

		onFrame();
		expect(trigger).not.toHaveBeenCalled();
	});

	it("forwards the second and subsequent calls", () => {
		const trigger = vi.fn();
		const onFrame = makeFirstFrameSkipper(trigger);

		onFrame();
		onFrame();
		onFrame();
		expect(trigger).toHaveBeenCalledTimes(2);
	});

	it("each instance has its own first-frame state", () => {
		const triggerA = vi.fn();
		const triggerB = vi.fn();
		const onFrameA = makeFirstFrameSkipper(triggerA);
		const onFrameB = makeFirstFrameSkipper(triggerB);

		onFrameA(); // skipped
		onFrameA(); // forwarded
		onFrameB(); // skipped (independent state)

		expect(triggerA).toHaveBeenCalledTimes(1);
		expect(triggerB).not.toHaveBeenCalled();
	});

	it("reset() rearms the skip guard so the next call is swallowed again", () => {
		// The page resets the guard whenever the stream reconnects after
		// `stream_expired` — the server replays state from scratch on the
		// new connection, so the first non-silent frame on the rebuilt
		// stream is again redundant with the SSR snapshot the load
		// function will re-fetch.
		const trigger = vi.fn();
		const onFrame = makeFirstFrameSkipper(trigger);

		onFrame(); // skipped
		onFrame(); // forwarded → 1
		onFrame.reset();
		onFrame(); // skipped again
		onFrame(); // forwarded → 2

		expect(trigger).toHaveBeenCalledTimes(2);
	});
});
