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
	type SafariConnectionRecoveryOptions,
} from "./safari-connection-recovery";

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
});
