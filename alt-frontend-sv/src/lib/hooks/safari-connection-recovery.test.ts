/**
 * Safari Connection Recovery Hook Tests
 *
 * Safari aggressively drops network connections when tabs are backgrounded for
 * power-saving. When the tab returns to foreground after extended idle, fetches
 * fail with "Could not connect to server" (NSURLErrorDomain -1004).
 *
 * This hook detects prolonged background periods and triggers TanStack Query
 * invalidation on tab return to refetch stale data.
 */

import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import {
	createSafariConnectionRecovery,
	isNetworkFailureError,
	performGuardedReload,
	type SafariConnectionRecoveryOptions,
} from "./safari-connection-recovery";

function makeFakeStorage(initial: Record<string, string> = {}) {
	const map = new Map<string, string>(Object.entries(initial));
	return {
		getItem: (k: string) => map.get(k) ?? null,
		setItem: (k: string, v: string) => {
			map.set(k, String(v));
		},
		removeItem: (k: string) => {
			map.delete(k);
		},
		_map: map,
	};
}

interface FakeDoc {
	addEventListener: (type: string, listener: EventListener) => void;
	removeEventListener: (type: string, listener: EventListener) => void;
	visibilityState: "visible" | "hidden";
}

function makeFakeDoc() {
	const listeners = new Map<string, Set<EventListener>>();
	const doc: FakeDoc = {
		visibilityState: "visible",
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
	return { doc, fire, listenerCount };
}

describe("createSafariConnectionRecovery", () => {
	let now: number;
	let fakeDoc: ReturnType<typeof makeFakeDoc>;

	beforeEach(() => {
		now = 1_000_000;
		fakeDoc = makeFakeDoc();
	});

	it("calls onRecoveryNeeded when hidden duration exceeds threshold", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");

		now += 60_000;

		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");

		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);
		expect(onRecoveryNeeded).toHaveBeenCalledWith({
			reason: "visibility",
			hiddenDurationMs: 60_000,
		});

		handle.dispose();
	});

	it("does NOT call onRecoveryNeeded for short background periods", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");

		now += 5_000;

		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");

		expect(onRecoveryNeeded).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("calls onRecoveryNeeded on bfcache restore (pageshow with persisted=true)", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		fakeDoc.fire("pageshow", { persisted: true });

		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);
		expect(onRecoveryNeeded).toHaveBeenCalledWith({
			reason: "bfcache",
			hiddenDurationMs: undefined,
		});

		handle.dispose();
	});

	it("ignores pageshow with persisted=false", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		fakeDoc.fire("pageshow", { persisted: false });

		expect(onRecoveryNeeded).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("calls onRecoveryNeeded on network reconnection", () => {
		const onRecoveryNeeded = vi.fn();
		const fakeNavigator = {
			onLine: false,
		};

		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			navigator: fakeNavigator as unknown as Navigator,
			window: {
				addEventListener: (type: string, listener: EventListener) => {
					if (type === "online") {
						fakeNavigator.onLine = true;
						listener(new Event("online"));
					}
				},
				removeEventListener: vi.fn(),
			} as unknown as Window,
		});

		expect(onRecoveryNeeded).toHaveBeenCalledWith({
			reason: "online",
			hiddenDurationMs: undefined,
		});

		handle.dispose();
	});

	it("dispose removes all listeners", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		expect(fakeDoc.listenerCount("visibilitychange")).toBe(1);
		expect(fakeDoc.listenerCount("pageshow")).toBe(1);

		handle.dispose();

		expect(fakeDoc.listenerCount("visibilitychange")).toBe(0);
		expect(fakeDoc.listenerCount("pageshow")).toBe(0);

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");
		now += 60_000;
		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");

		expect(onRecoveryNeeded).not.toHaveBeenCalled();
	});

	it("handles multiple hide/show cycles correctly", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
		});

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");
		now += 5_000;
		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");
		expect(onRecoveryNeeded).not.toHaveBeenCalled();

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");
		now += 60_000;
		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");
		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);

		fakeDoc.doc.visibilityState = "hidden";
		fakeDoc.fire("visibilitychange");
		now += 45_000;
		fakeDoc.doc.visibilityState = "visible";
		fakeDoc.fire("visibilitychange");
		expect(onRecoveryNeeded).toHaveBeenCalledTimes(2);

		handle.dispose();
	});

	it("calls onRecoveryNeeded on Page Lifecycle resume after a long freeze", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			storage: makeFakeStorage(),
		});

		fakeDoc.fire("freeze");
		now += 120_000;
		fakeDoc.fire("resume");

		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);
		expect(onRecoveryNeeded).toHaveBeenCalledWith({
			reason: "resume",
			hiddenDurationMs: 120_000,
		});

		handle.dispose();
	});

	it("does NOT call onRecoveryNeeded on resume after a short freeze", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			storage: makeFakeStorage(),
		});

		fakeDoc.fire("freeze");
		now += 5_000;
		fakeDoc.fire("resume");

		expect(onRecoveryNeeded).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("recovers on a non-persisted pageshow when sessionStorage shows a long background (Safari reloaded a discarded tab)", () => {
		const onRecoveryNeeded = vi.fn();
		// Simulate a hidden marker written by a previous page instance.
		const storage = makeFakeStorage({
			"alt:safari-recovery:hidden-at": String(now),
		});
		now += 300_000;
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			storage,
		});

		fakeDoc.fire("pageshow", { persisted: false });

		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);
		expect(onRecoveryNeeded).toHaveBeenCalledWith({
			reason: "resume",
			hiddenDurationMs: 300_000,
		});
		// Marker is consumed so a later pageshow does not re-trigger.
		fakeDoc.fire("pageshow", { persisted: false });
		expect(onRecoveryNeeded).toHaveBeenCalledTimes(1);

		handle.dispose();
	});

	it("does NOT recover on a non-persisted pageshow without a stored hidden marker", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			storage: makeFakeStorage(),
		});

		fakeDoc.fire("pageshow", { persisted: false });

		expect(onRecoveryNeeded).not.toHaveBeenCalled();

		handle.dispose();
	});

	it("dispose removes freeze and resume listeners too", () => {
		const onRecoveryNeeded = vi.fn();
		const handle = createSafariConnectionRecovery({
			thresholdMs: 30_000,
			onRecoveryNeeded,
			getNow: () => now,
			document: fakeDoc.doc as unknown as Document,
			storage: makeFakeStorage(),
		});

		expect(fakeDoc.listenerCount("freeze")).toBe(1);
		expect(fakeDoc.listenerCount("resume")).toBe(1);

		handle.dispose();

		expect(fakeDoc.listenerCount("freeze")).toBe(0);
		expect(fakeDoc.listenerCount("resume")).toBe(0);

		fakeDoc.fire("freeze");
		now += 120_000;
		fakeDoc.fire("resume");
		expect(onRecoveryNeeded).not.toHaveBeenCalled();
	});
});

describe("isNetworkFailureError", () => {
	it("recognises Safari/Chromium/Firefox network failures", () => {
		expect(isNetworkFailureError(new TypeError("Load failed"))).toBe(true);
		expect(isNetworkFailureError(new TypeError("Failed to fetch"))).toBe(true);
		expect(
			isNetworkFailureError(
				new TypeError("NetworkError when attempting to fetch resource."),
			),
		).toBe(true);
	});

	it("ignores aborts, HTTP/app errors, and non-errors", () => {
		const abort = Object.assign(new Error("aborted"), { name: "AbortError" });
		expect(isNetworkFailureError(abort)).toBe(false);
		expect(isNetworkFailureError(new TypeError("x is not a function"))).toBe(
			false,
		);
		expect(isNetworkFailureError(new Error("Load failed"))).toBe(false);
		expect(isNetworkFailureError("Load failed")).toBe(false);
		expect(isNetworkFailureError(undefined)).toBe(false);
	});
});

describe("performGuardedReload", () => {
	it("reloads once and records the timestamp", () => {
		const reload = vi.fn();
		const storage = makeFakeStorage();
		const fakeWin = { location: { reload }, sessionStorage: undefined };

		const result = performGuardedReload({
			window: fakeWin as unknown as Window,
			storage,
			getNow: () => 1_000_000,
			cooldownMs: 60_000,
		});

		expect(result).toBe(true);
		expect(reload).toHaveBeenCalledTimes(1);
		expect(storage.getItem("alt:safari-recovery:last-reload-at")).toBe(
			"1000000",
		);
	});

	it("does not reload again within the cooldown window", () => {
		const reload = vi.fn();
		const storage = makeFakeStorage({
			"alt:safari-recovery:last-reload-at": "1000000",
		});
		const fakeWin = { location: { reload } };

		const result = performGuardedReload({
			window: fakeWin as unknown as Window,
			storage,
			getNow: () => 1_030_000, // 30s later, cooldown is 60s
			cooldownMs: 60_000,
		});

		expect(result).toBe(false);
		expect(reload).not.toHaveBeenCalled();
	});

	it("reloads again once the cooldown has elapsed", () => {
		const reload = vi.fn();
		const storage = makeFakeStorage({
			"alt:safari-recovery:last-reload-at": "1000000",
		});
		const fakeWin = { location: { reload } };

		const result = performGuardedReload({
			window: fakeWin as unknown as Window,
			storage,
			getNow: () => 1_120_000, // 120s later
			cooldownMs: 60_000,
		});

		expect(result).toBe(true);
		expect(reload).toHaveBeenCalledTimes(1);
	});

	it("returns false when there is no usable location.reload", () => {
		expect(performGuardedReload({ window: {} as unknown as Window })).toBe(
			false,
		);
	});
});
