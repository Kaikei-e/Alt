import { describe, it, expect } from "vitest";
import { createDwellTracker } from "./useTrailOutcome.svelte";

// The dwell tracker is the framework-free core of useTrailOutcome: it
// accumulates visible time (Page Visibility pauses) and guarantees a single
// flush — the PM-2026-045 emit-ownership lesson, enforced in the data type.
describe("createDwellTracker", () => {
	function fakeClock(start = 0) {
		let t = start;
		return {
			now: () => t,
			advance: (ms: number) => {
				t += ms;
			},
		};
	}

	it("accumulates visible time between start and flush", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		tracker.start();
		clock.advance(5000);
		expect(tracker.flush()).toBe(5000);
	});

	it("excludes hidden time (pause/resume)", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		tracker.start();
		clock.advance(2000);
		tracker.pause();
		clock.advance(60000);
		tracker.start();
		clock.advance(1000);
		expect(tracker.flush()).toBe(3000);
	});

	it("flushes exactly once — later flushes return null", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		tracker.start();
		clock.advance(100);
		expect(tracker.flush()).toBe(100);
		clock.advance(100);
		expect(tracker.flush()).toBeNull();
	});

	it("a start after flush does not restart measurement", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		tracker.start();
		clock.advance(100);
		tracker.flush();
		tracker.start();
		clock.advance(500);
		expect(tracker.flush()).toBeNull();
	});

	it("flush before any visible time reports zero", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		expect(tracker.flush()).toBe(0);
	});

	it("double start does not double-count", () => {
		const clock = fakeClock();
		const tracker = createDwellTracker(clock.now);
		tracker.start();
		clock.advance(1000);
		tracker.start();
		clock.advance(1000);
		expect(tracker.flush()).toBe(2000);
	});
});
