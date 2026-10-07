import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { mockBeforeNavigate, mockEmitTrailOutcome, mockOnDestroy } = vi.hoisted(
	() => ({
		mockBeforeNavigate: vi.fn(),
		mockEmitTrailOutcome: vi.fn(),
		mockOnDestroy: vi.fn(),
	}),
);

vi.mock("svelte", async (importOriginal) => {
	const actual = await importOriginal<typeof import("svelte")>();
	return {
		...actual,
		onDestroy: mockOnDestroy,
	};
});

vi.mock("$app/env", () => ({ browser: true }));
vi.mock("$app/navigation", () => ({
	beforeNavigate: mockBeforeNavigate,
}));
vi.mock("$app/paths", () => ({
	resolve: (path: string) => `/${path.replace(/^\//, "")}`,
}));
vi.mock("#lib/connect/knowledge_trail.js", () => ({
	emitTrailOutcome: mockEmitTrailOutcome,
}));
vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: vi.fn(() => ({})),
}));

import { createDwellTracker, useTrailOutcome } from "./useTrailOutcome.svelte";

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

describe("useTrailOutcome shallow navigation guard", () => {
	let beforeNavigateCallback: ((nav: { shallow: boolean }) => void) | null =
		null;

	beforeEach(() => {
		vi.clearAllMocks();
		mockEmitTrailOutcome.mockResolvedValue({});
		mockBeforeNavigate.mockImplementation((cb) => {
			beforeNavigateCallback = cb;
		});

		vi.stubGlobal("document", {
			visibilityState: "visible",
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
		});
		vi.stubGlobal("window", {
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
		});
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("ignores shallow navigation and does not flush dwell outcome", () => {
		useTrailOutcome("branch-1", "article:123");

		expect(beforeNavigateCallback).not.toBeNull();
		beforeNavigateCallback!({ shallow: true });

		expect(mockEmitTrailOutcome).not.toHaveBeenCalled();
	});

	it("flushes dwell outcome on non-shallow navigation", () => {
		useTrailOutcome("branch-1", "article:123");

		expect(beforeNavigateCallback).not.toBeNull();
		beforeNavigateCallback!({ shallow: false });

		expect(mockEmitTrailOutcome).toHaveBeenCalledTimes(1);
	});
});
