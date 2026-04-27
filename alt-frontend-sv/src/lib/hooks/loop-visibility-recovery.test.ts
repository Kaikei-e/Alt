import { describe, expect, it, vi } from "vitest";
import { startVisibilityRecovery } from "./loop-visibility-recovery.ts";

/**
 * Tab-return recovery for the Knowledge Loop hook.
 *
 * After backgrounding the tab for longer than the threshold, an in-flight
 * `/loop/transition` request can be left dangling (server JWT expiry,
 * connection dropped, bfcache freeze). The visibility-recovery hook fires
 * `onRecover` so the page can call `loop.resetInFlight("visibility")`.
 *
 * Quick blurs (under threshold) MUST NOT trigger recovery — they happen on
 * normal alt-tab and we don't want to clear in-flight state for fetches that
 * are about to resolve.
 */

interface FakeDoc {
	addEventListener: (type: string, listener: EventListener) => void;
	removeEventListener: (type: string, listener: EventListener) => void;
	visibilityState: "visible" | "hidden";
}

function makeFakeDoc() {
	const listeners = new Map<string, Set<EventListener>>();
	const doc = {
		visibilityState: "visible" as "visible" | "hidden",
		addEventListener(type: string, listener: EventListener) {
			let set = listeners.get(type);
			if (!set) {
				set = new Set();
				listeners.set(type, set);
			}
			set.add(listener);
		},
		removeEventListener(type: string, listener: EventListener) {
			listeners.get(type)?.delete(listener);
		},
	};
	const fire = (type: string, evt?: unknown) => {
		const set = listeners.get(type);
		if (!set) return;
		for (const l of set) l(evt as Event);
	};
	const listenerCount = (type: string) => listeners.get(type)?.size ?? 0;
	return { doc: doc as FakeDoc, fire, listenerCount };
}

describe("startVisibilityRecovery", () => {
	it("calls onRecover when hidden duration exceeds threshold (visibilitychange)", () => {
		const onRecover = vi.fn();
		let now = 1_000_000;
		const { doc, fire } = makeFakeDoc();

		const handle = startVisibilityRecovery({
			thresholdMs: 30_000,
			onRecover,
			getNow: () => now,
			document: doc as unknown as Document,
		});

		// Tab goes hidden.
		doc.visibilityState = "hidden";
		fire("visibilitychange");

		// 31 seconds pass.
		now += 31_000;

		// Tab becomes visible again.
		doc.visibilityState = "visible";
		fire("visibilitychange");

		expect(onRecover).toHaveBeenCalledTimes(1);
		expect(onRecover).toHaveBeenCalledWith({ reason: "visibility" });

		handle.dispose();
	});

	it("does NOT call onRecover on a quick blur under the threshold", () => {
		const onRecover = vi.fn();
		let now = 0;
		const { doc, fire } = makeFakeDoc();

		const handle = startVisibilityRecovery({
			thresholdMs: 30_000,
			onRecover,
			getNow: () => now,
			document: doc as unknown as Document,
		});

		doc.visibilityState = "hidden";
		fire("visibilitychange");

		now += 5_000;

		doc.visibilityState = "visible";
		fire("visibilitychange");

		expect(onRecover).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("calls onRecover from pageshow with persisted=true (bfcache restore)", () => {
		const onRecover = vi.fn();
		const { doc, fire } = makeFakeDoc();

		const handle = startVisibilityRecovery({
			thresholdMs: 30_000,
			onRecover,
			getNow: () => 0,
			document: doc as unknown as Document,
		});

		// bfcache restore — the page wasn't unloaded, so visibilitychange may
		// not fire reliably. pageshow with persisted=true is the canonical
		// signal that we resumed from a frozen state.
		fire("pageshow", { persisted: true });

		expect(onRecover).toHaveBeenCalledTimes(1);
		expect(onRecover).toHaveBeenCalledWith({ reason: "bfcache" });

		handle.dispose();
	});

	it("ignores pageshow with persisted=false (fresh load)", () => {
		const onRecover = vi.fn();
		const { doc, fire } = makeFakeDoc();

		const handle = startVisibilityRecovery({
			thresholdMs: 30_000,
			onRecover,
			getNow: () => 0,
			document: doc as unknown as Document,
		});

		fire("pageshow", { persisted: false });
		expect(onRecover).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("dispose removes both visibilitychange and pageshow listeners", () => {
		const onRecover = vi.fn();
		const { doc, fire, listenerCount } = makeFakeDoc();

		const handle = startVisibilityRecovery({
			thresholdMs: 30_000,
			onRecover,
			getNow: () => 0,
			document: doc as unknown as Document,
		});

		expect(listenerCount("visibilitychange")).toBe(1);
		expect(listenerCount("pageshow")).toBe(1);

		handle.dispose();

		expect(listenerCount("visibilitychange")).toBe(0);
		expect(listenerCount("pageshow")).toBe(0);

		// Post-dispose events are no-ops.
		fire("visibilitychange");
		fire("pageshow", { persisted: true });
		expect(onRecover).not.toHaveBeenCalled();
	});
});
