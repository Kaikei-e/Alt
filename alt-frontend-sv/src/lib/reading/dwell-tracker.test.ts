import { afterEach, describe, expect, it, vi } from "vitest";
import { createDwellTracker, type VisibilitySource } from "./dwell-tracker";

function createFakeClock(initial = 0) {
	let current = initial;
	return {
		now: () => current,
		advance(ms: number) {
			current += ms;
		},
	};
}

function createFakeVisibility(
	initial: DocumentVisibilityState = "visible",
): VisibilitySource & {
	setState(next: DocumentVisibilityState): void;
	listenerCount(): number;
} {
	let state = initial;
	const listeners = new Set<() => void>();
	return {
		get visibilityState() {
			return state;
		},
		addEventListener(_type: "visibilitychange", listener: () => void) {
			listeners.add(listener);
		},
		removeEventListener(_type: "visibilitychange", listener: () => void) {
			listeners.delete(listener);
		},
		setState(next: DocumentVisibilityState) {
			state = next;
			for (const listener of listeners) {
				listener();
			}
		},
		listenerCount() {
			return listeners.size;
		},
	};
}

describe("createDwellTracker", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it("accumulates foreground time while visible", () => {
		const clock = createFakeClock(1000);
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		clock.advance(4250);

		expect(tracker.elapsedMs()).toBe(4250);
		expect(tracker.stop()).toBe(4250);
	});

	it("works with fake timers and the default clock", () => {
		vi.useFakeTimers();
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ visibility });

		vi.advanceTimersByTime(1500);
		visibility.setState("hidden");
		vi.advanceTimersByTime(8000);
		visibility.setState("visible");
		vi.advanceTimersByTime(500);

		expect(tracker.stop()).toBe(2000);
	});

	it("pauses while hidden and resumes when visible again", () => {
		const clock = createFakeClock();
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		clock.advance(2000);
		visibility.setState("hidden");
		clock.advance(30000);
		expect(tracker.elapsedMs()).toBe(2000);

		visibility.setState("visible");
		clock.advance(1200);
		visibility.setState("hidden");
		clock.advance(5000);
		visibility.setState("visible");
		clock.advance(800);

		expect(tracker.stop()).toBe(4000);
	});

	it("starts paused when created in a hidden tab until the tab becomes visible", () => {
		const clock = createFakeClock();
		const visibility = createFakeVisibility("hidden");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		clock.advance(10000);
		expect(tracker.elapsedMs()).toBe(0);

		visibility.setState("visible");
		clock.advance(750);

		expect(tracker.stop()).toBe(750);
	});

	it("does not double-count on repeated start or start while hidden", () => {
		const clock = createFakeClock();
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		clock.advance(1000);
		tracker.start();
		clock.advance(1000);

		visibility.setState("hidden");
		tracker.start();
		clock.advance(5000);

		expect(tracker.stop()).toBe(2000);
	});

	it("supports manual pause and start while visible", () => {
		const clock = createFakeClock();
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		clock.advance(600);
		tracker.pause();
		clock.advance(3000);
		tracker.start();
		clock.advance(400);

		expect(tracker.stop()).toBe(1000);
	});

	it("detaches the visibility listener on stop and stays idempotent", () => {
		const clock = createFakeClock();
		const visibility = createFakeVisibility("visible");
		const tracker = createDwellTracker({ now: clock.now, visibility });

		expect(visibility.listenerCount()).toBe(1);
		clock.advance(900);

		expect(tracker.stop()).toBe(900);
		expect(visibility.listenerCount()).toBe(0);

		clock.advance(5000);
		visibility.setState("hidden");
		visibility.setState("visible");
		tracker.start();
		clock.advance(5000);

		expect(tracker.stop()).toBe(900);
		expect(tracker.elapsedMs()).toBe(900);
	});

	it("clamps negative clock jumps to zero", () => {
		const clock = createFakeClock(5000);
		const tracker = createDwellTracker({ now: clock.now, visibility: null });

		clock.advance(-1000);

		expect(tracker.stop()).toBe(0);
	});
});
