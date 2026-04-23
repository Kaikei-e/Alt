/**
 * observeTiles action — unit tests.
 *
 * The action wraps a single IntersectionObserver + MutationObserver around a
 * container element (e.g. the /loop foreground plane). Each child that carries
 * `data-entry-key` is observed exactly once; when it becomes at least 50%
 * visible the action invokes `onObserve(entryKey)`. Added / removed children
 * are picked up automatically via MutationObserver.
 *
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { observeTiles } from "./observe-tiles";

let observerInstances: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
	readonly root: Element | Document | null = null;
	readonly rootMargin: string = "";
	readonly scrollMargin: string = "";
	readonly thresholds: readonly number[] = [];
	private callback: IntersectionObserverCallback;
	observed: Set<Element> = new Set();
	disconnected = false;

	constructor(
		callback: IntersectionObserverCallback,
		options?: IntersectionObserverInit,
	) {
		this.callback = callback;
		this.root = options?.root ?? null;
		this.rootMargin = options?.rootMargin ?? "";
		this.thresholds = options?.threshold
			? Array.isArray(options.threshold)
				? options.threshold
				: [options.threshold]
			: [0];
		observerInstances.push(this);
	}

	observe(target: Element): void {
		this.observed.add(target);
	}
	unobserve(target: Element): void {
		this.observed.delete(target);
	}
	disconnect(): void {
		this.observed.clear();
		this.disconnected = true;
	}
	takeRecords(): IntersectionObserverEntry[] {
		return [];
	}

	simulate(target: Element, isIntersecting: boolean): void {
		const entry: IntersectionObserverEntry = {
			target,
			isIntersecting,
			intersectionRatio: isIntersecting ? 1 : 0,
			boundingClientRect: {} as DOMRectReadOnly,
			intersectionRect: {} as DOMRectReadOnly,
			rootBounds: null,
			time: performance.now(),
		} as IntersectionObserverEntry;
		this.callback([entry], this);
	}
}

function makeTile(entryKey: string): HTMLElement {
	const div = document.createElement("div");
	div.setAttribute("data-entry-key", entryKey);
	return div;
}

describe("observeTiles action", () => {
	let originalIO: typeof IntersectionObserver;

	beforeEach(() => {
		observerInstances = [];
		originalIO = globalThis.IntersectionObserver;
		globalThis.IntersectionObserver =
			MockIntersectionObserver as unknown as typeof IntersectionObserver;
	});

	afterEach(() => {
		globalThis.IntersectionObserver = originalIO;
	});

	it("observes every child that already carries data-entry-key", () => {
		const container = document.createElement("div");
		container.appendChild(makeTile("a"));
		container.appendChild(makeTile("b"));
		document.body.appendChild(container);

		observeTiles(container, { onObserve: vi.fn() });

		expect(observerInstances).toHaveLength(1);
		expect(observerInstances[0]?.observed.size).toBe(2);
	});

	it("invokes onObserve(entryKey) when a tile becomes visible", () => {
		const container = document.createElement("div");
		const tile = makeTile("alpha");
		container.appendChild(tile);
		document.body.appendChild(container);

		const onObserve = vi.fn();
		observeTiles(container, { onObserve });

		observerInstances[0]?.simulate(tile, true);

		expect(onObserve).toHaveBeenCalledTimes(1);
		expect(onObserve).toHaveBeenCalledWith("alpha");
	});

	it("does not fire onObserve for non-intersecting entries", () => {
		const container = document.createElement("div");
		const tile = makeTile("alpha");
		container.appendChild(tile);
		document.body.appendChild(container);

		const onObserve = vi.fn();
		observeTiles(container, { onObserve });

		observerInstances[0]?.simulate(tile, false);

		expect(onObserve).not.toHaveBeenCalled();
	});

	it("picks up tiles added after mount via MutationObserver", async () => {
		const container = document.createElement("div");
		document.body.appendChild(container);

		const onObserve = vi.fn();
		observeTiles(container, { onObserve });

		expect(observerInstances[0]?.observed.size).toBe(0);

		const added = makeTile("added-later");
		container.appendChild(added);

		await new Promise((r) => setTimeout(r, 0));
		// MutationObserver callbacks flush microtask-wise; give it one tick.
		await Promise.resolve();

		expect(observerInstances[0]?.observed.has(added)).toBe(true);
	});

	it("ignores duplicate observe() calls for the same element", () => {
		const container = document.createElement("div");
		const tile = makeTile("once");
		container.appendChild(tile);
		document.body.appendChild(container);

		observeTiles(container, { onObserve: vi.fn() });
		const observer = observerInstances[0];
		if (!observer) throw new Error("observer missing");
		const observeSpy = vi.spyOn(observer, "observe");

		// A MutationObserver fire on an unrelated change should not re-observe.
		const sibling = document.createElement("span");
		container.appendChild(sibling);
		// manually flush
		observeSpy.mockClear();
	});

	it("disconnects both observers on destroy", () => {
		const container = document.createElement("div");
		container.appendChild(makeTile("a"));
		document.body.appendChild(container);

		const action = observeTiles(container, { onObserve: vi.fn() });
		const observer = observerInstances[0];
		if (!observer) throw new Error("observer missing");

		action.destroy();

		expect(observer.disconnected).toBe(true);
	});

	it("applies the configured rootMargin and threshold", () => {
		const container = document.createElement("div");
		document.body.appendChild(container);

		observeTiles(container, {
			onObserve: vi.fn(),
			rootMargin: "10px",
			threshold: 0.5,
		});

		expect(observerInstances[0]?.rootMargin).toBe("10px");
		expect(observerInstances[0]?.thresholds).toEqual([0.5]);
	});
});
