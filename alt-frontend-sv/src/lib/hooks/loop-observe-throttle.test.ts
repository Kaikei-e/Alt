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
