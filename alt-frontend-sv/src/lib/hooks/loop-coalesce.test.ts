import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeCoalescedRefresh } from "./loop-coalesce";

/**
 * Coalesced refresh helper for the Knowledge Loop stream.
 *
 * Production regression (2026-04-26 nginx + alt-butterfly-facade logs):
 * `useKnowledgeLoopStream` callbacks fired `invalidateAll()` unconditionally
 * for every non-heartbeat frame and every JWT expiry, which produced a
 * positive-feedback storm of `__data.json?x-sveltekit-invalidated=1` requests
 * that exhausted the browser per-origin connection ceiling
 * (`ERR_INSUFFICIENT_RESOURCES`).
 *
 * The contract this helper enforces:
 *   1. Trailing-edge debounce (default 600 ms): bursts collapse to one call
 *      after the last trigger settles within the window.
 *   2. Single-flight guard: while one underlying refresh is in flight, all
 *      additional triggers collapse into exactly one trailing follow-up that
 *      runs after the current flight completes.
 *   3. Cancellation: `dispose()` aborts any pending timer and any pending
 *      trailing follow-up. In-flight calls cannot be aborted (they are owned
 *      by the caller-supplied async fn) but their result is ignored.
 *
 * The helper is pure (no Svelte runes), takes a clock and a scheduler so it
 * can be exercised deterministically with `vi.useFakeTimers`.
 */

describe("makeCoalescedRefresh — debounced + single-flight stream refresh", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it("collapses N rapid triggers within the window into a single call", async () => {
		const refresh = vi.fn(async () => {});
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		for (let i = 0; i < 50; i += 1) coalesce.trigger();
		expect(refresh).not.toHaveBeenCalled();

		await vi.advanceTimersByTimeAsync(600);
		expect(refresh).toHaveBeenCalledTimes(1);
	});

	it("each new trigger inside the window resets the debounce (trailing edge)", async () => {
		const refresh = vi.fn(async () => {});
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(500);
		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(500);
		expect(refresh).not.toHaveBeenCalled();

		await vi.advanceTimersByTimeAsync(100);
		expect(refresh).toHaveBeenCalledTimes(1);
	});

	it("triggers arriving while a refresh is in-flight collapse to exactly one trailing follow-up", async () => {
		let resolveInner!: () => void;
		const refresh = vi.fn(
			() =>
				new Promise<void>((resolve) => {
					resolveInner = resolve;
				}),
		);
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		// First trigger → debounce → flight starts.
		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(600);
		expect(refresh).toHaveBeenCalledTimes(1);

		// 20 more triggers arrive while the first call is still in flight. With
		// the single-flight guard, none of them spawn another refresh; instead
		// they collapse into one trailing follow-up scheduled after the in-flight
		// call settles.
		for (let i = 0; i < 20; i += 1) coalesce.trigger();
		expect(refresh).toHaveBeenCalledTimes(1);

		// Settle the first call; the trailing follow-up is then scheduled.
		resolveInner();
		await vi.advanceTimersByTimeAsync(0);
		await vi.advanceTimersByTimeAsync(600);
		expect(refresh).toHaveBeenCalledTimes(2);
	});

	it("dispose() cancels any pending debounce timer", async () => {
		const refresh = vi.fn(async () => {});
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		coalesce.trigger();
		coalesce.dispose();

		await vi.advanceTimersByTimeAsync(2_000);
		expect(refresh).not.toHaveBeenCalled();
	});

	it("dispose() cancels a trailing follow-up scheduled while a refresh was in flight", async () => {
		let resolveInner!: () => void;
		const refresh = vi.fn(
			() =>
				new Promise<void>((resolve) => {
					resolveInner = resolve;
				}),
		);
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(600);
		expect(refresh).toHaveBeenCalledTimes(1);

		coalesce.trigger();
		coalesce.dispose();

		resolveInner();
		await vi.advanceTimersByTimeAsync(2_000);
		expect(refresh).toHaveBeenCalledTimes(1);
	});

	it("a refresh that throws does not break the coalescer (next trigger still fires)", async () => {
		let calls = 0;
		const refresh = vi.fn(async () => {
			calls += 1;
			if (calls === 1) throw new Error("boom");
		});
		const coalesce = makeCoalescedRefresh(refresh, { windowMs: 600 });

		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(600);
		// One in-flight call settled (rejected). State must be cleared.
		await vi.advanceTimersByTimeAsync(0);

		coalesce.trigger();
		await vi.advanceTimersByTimeAsync(600);
		expect(calls).toBe(2);
	});
});
