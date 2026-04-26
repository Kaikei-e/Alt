import { describe, it, expect } from "vitest";
import { makeObserveThrottle } from "./loop-observe-throttle";

const MIN_MS = 60_000;

describe("makeObserveThrottle — 60s per entry gate (ADR-000831 §8.2)", () => {
	it("allows the first observe for an entry", () => {
		const t = makeObserveThrottle(MIN_MS);
		expect(t.shouldEmit("article:42", 0)).toBe(true);
	});

	it("rejects a repeat within the window", () => {
		const t = makeObserveThrottle(MIN_MS);
		t.shouldEmit("article:42", 0);
		expect(t.shouldEmit("article:42", 59_999)).toBe(false);
	});

	it("allows a repeat after the window", () => {
		const t = makeObserveThrottle(MIN_MS);
		t.shouldEmit("article:42", 0);
		expect(t.shouldEmit("article:42", 60_001)).toBe(true);
	});

	it("keeps throttle state independent across entry keys", () => {
		const t = makeObserveThrottle(MIN_MS);
		t.shouldEmit("article:42", 0);
		expect(t.shouldEmit("article:43", 100)).toBe(true);
		expect(t.shouldEmit("article:42", 100)).toBe(false);
	});

	it("reset() clears the throttle for that entry only", () => {
		const t = makeObserveThrottle(MIN_MS);
		t.shouldEmit("article:42", 0);
		t.shouldEmit("article:43", 0);
		t.reset("article:42");
		expect(t.shouldEmit("article:42", 100)).toBe(true);
		expect(t.shouldEmit("article:43", 100)).toBe(false);
	});
});

describe("makeObserveThrottle — Storage persistence (ADR-000846 follow-up)", () => {
	function makeFakeStorage() {
		const store = new Map<string, string>();
		return {
			store,
			getItem: (k: string) => store.get(k) ?? null,
			setItem: (k: string, v: string) => {
				store.set(k, v);
			},
			removeItem: (k: string) => {
				store.delete(k);
			},
		};
	}

	it("persists last-fired timestamps to Storage so reload-equivalent rebuilds keep the throttle", () => {
		// First "page session": fire once for article:42 at t=0, recording the
		// timestamp into Storage.
		const storage = makeFakeStorage();
		const t1 = makeObserveThrottle(MIN_MS, { storage });
		expect(t1.shouldEmit("article:42", 0)).toBe(true);

		// Simulate page reload: a fresh throttle constructed against the same
		// Storage MUST honour the prior timestamp and refuse a re-fire within
		// the 60s window. Without this, every reload within the backend §8.4
		// rate-limit window produces a guaranteed 429.
		const t2 = makeObserveThrottle(MIN_MS, { storage });
		expect(t2.shouldEmit("article:42", 30_000)).toBe(false);
		expect(t2.shouldEmit("article:42", 60_001)).toBe(true);
	});

	it("falls back to in-memory when Storage is null", () => {
		const t = makeObserveThrottle(MIN_MS, { storage: null });
		expect(t.shouldEmit("article:42", 0)).toBe(true);
		expect(t.shouldEmit("article:42", 1000)).toBe(false);
	});

	it("survives a corrupt Storage payload by treating the map as empty", () => {
		const storage = makeFakeStorage();
		storage.store.set("alt:loop:observe-throttle:v1", "not-json{{{");
		const t = makeObserveThrottle(MIN_MS, { storage });
		// Corrupt blob → empty map → first emit allowed.
		expect(t.shouldEmit("article:42", 0)).toBe(true);
	});

	it("treats Storage throws as graceful in-memory degradation", () => {
		const throwingStorage = {
			getItem: () => {
				throw new Error("quota exceeded");
			},
			setItem: () => {
				throw new Error("quota exceeded");
			},
			removeItem: () => {},
		};
		const t = makeObserveThrottle(MIN_MS, { storage: throwingStorage });
		// Throws on read → empty map → first emit allowed; setItem throw is
		// swallowed and the in-memory throttle still gates the second emit.
		expect(t.shouldEmit("article:42", 0)).toBe(true);
		expect(t.shouldEmit("article:42", 1000)).toBe(false);
	});

	it("reset() clears Storage too so a subsequent emit is allowed", () => {
		const storage = makeFakeStorage();
		const t = makeObserveThrottle(MIN_MS, { storage });
		t.shouldEmit("article:42", 0);
		t.reset("article:42");
		// Reload-equivalent: fresh throttle against same Storage must allow
		// emit because the entry was reset.
		const t2 = makeObserveThrottle(MIN_MS, { storage });
		expect(t2.shouldEmit("article:42", 100)).toBe(true);
	});
});
