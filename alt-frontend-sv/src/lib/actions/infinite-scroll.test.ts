/**
 * Infinite Scroll Action Tests
 *
 * Tests for the IntersectionObserver-based infinite scroll action
 *
 * @vitest-environment jsdom
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { infiniteScroll } from "./infinite-scroll";

// Mock IntersectionObserver
let mockObserverInstances: MockIntersectionObserver[] = [];

class MockIntersectionObserver implements IntersectionObserver {
	readonly root: Element | Document | null = null;
	readonly rootMargin: string = "";
	readonly scrollMargin: string = "";
	readonly thresholds: readonly number[] = [];
	private callback: IntersectionObserverCallback;
	private elements: Set<Element> = new Set();

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
		mockObserverInstances.push(this);
	}

	observe(target: Element): void {
		this.elements.add(target);
	}

	unobserve(target: Element): void {
		this.elements.delete(target);
	}

	disconnect(): void {
		this.elements.clear();
	}

	takeRecords(): IntersectionObserverEntry[] {
		return [];
	}

	isObserving(target: Element): boolean {
		return this.elements.has(target);
	}

	// Test helper: simulate intersection
	simulateIntersection(isIntersecting: boolean): void {
		const entries: IntersectionObserverEntry[] = Array.from(this.elements).map(
			(element) =>
				({
					target: element,
					isIntersecting,
					intersectionRatio: isIntersecting ? 1 : 0,
					boundingClientRect: {} as DOMRectReadOnly,
					intersectionRect: {} as DOMRectReadOnly,
					rootBounds: null,
					time: performance.now(),
				}) as IntersectionObserverEntry,
		);
		this.callback(entries, this);
	}
}

describe("infiniteScroll action", () => {
	let originalIntersectionObserver: typeof IntersectionObserver | undefined;
	let element: HTMLDivElement;

	beforeEach(() => {
		mockObserverInstances = [];
		originalIntersectionObserver = globalThis.IntersectionObserver;
		globalThis.IntersectionObserver =
			MockIntersectionObserver as unknown as typeof IntersectionObserver;
		element = document.createElement("div");
		document.body.appendChild(element);
	});

	afterEach(() => {
		element.remove();
		if (originalIntersectionObserver) {
			globalThis.IntersectionObserver = originalIntersectionObserver;
		}
	});

	it("should create an IntersectionObserver when initialized", () => {
		const callback = vi.fn();

		infiniteScroll(element, { callback });

		expect(mockObserverInstances.length).toBe(1);
	});

	it("should call callback when element intersects", async () => {
		const callback = vi.fn();

		infiniteScroll(element, { callback });

		// Simulate intersection
		mockObserverInstances[0]?.simulateIntersection(true);

		// Wait for async operations
		await vi.waitFor(() => {
			expect(callback).toHaveBeenCalledTimes(1);
		});
	});

	it("should unobserve element before callback resolves and re-observe inside requestAnimationFrame after callback resolves", async () => {
		let resolveCallback!: () => void;
		const callback = vi.fn(
			() =>
				new Promise<void>((resolve) => {
					resolveCallback = resolve;
				}),
		);

		let rafCallback = null as FrameRequestCallback | null;
		const rafSpy = vi
			.spyOn(globalThis, "requestAnimationFrame")
			.mockImplementation((cb: FrameRequestCallback) => {
				rafCallback = cb;
				return 1;
			});

		infiniteScroll(element, { callback });

		const observer = mockObserverInstances[0];
		if (!observer) throw new Error("Observer should exist");
		const unobserveSpy = vi.spyOn(observer, "unobserve");
		const observeSpy = vi.spyOn(observer, "observe");

		// Trigger intersection
		observer.simulateIntersection(true);

		// Before callback resolves, element must be unobserved
		expect(unobserveSpy).toHaveBeenCalledWith(element);
		expect(observer.isObserving(element)).toBe(false);
		expect(observeSpy).not.toHaveBeenCalled();

		// Resolve async callback
		resolveCallback();
		await vi.waitFor(() => {
			expect(rafCallback).not.toBeNull();
		});

		// Still not re-observed until requestAnimationFrame fires
		expect(observeSpy).not.toHaveBeenCalled();

		// Fire requestAnimationFrame
		rafCallback?.(performance.now());
		await vi.waitFor(() => {
			expect(observeSpy).toHaveBeenCalledWith(element);
		});
		expect(observer.isObserving(element)).toBe(true);

		rafSpy.mockRestore();
	});

	it("should not call callback when element does not intersect", () => {
		const callback = vi.fn();

		infiniteScroll(element, { callback });

		// Simulate non-intersection
		mockObserverInstances[0]?.simulateIntersection(false);

		expect(callback).not.toHaveBeenCalled();
	});

	it("should not create observer when disabled is true", () => {
		const callback = vi.fn();

		infiniteScroll(element, { callback, disabled: true });

		// Observer should not be created when disabled
		expect(mockObserverInstances.length).toBe(0);
	});

	it("should pass correct rootMargin and threshold to IntersectionObserver", () => {
		const rootElement = document.createElement("div");
		const callback = vi.fn();

		infiniteScroll(element, {
			callback,
			root: rootElement,
			rootMargin: "100px",
			threshold: 0.5,
		});

		const observer = mockObserverInstances[0];
		expect(observer).toBeDefined();
		expect(observer?.rootMargin).toBe("100px");
		expect(observer?.thresholds).toEqual([0.5]);
	});

	it("should disconnect observer on destroy", () => {
		const callback = vi.fn();

		const action = infiniteScroll(element, { callback });
		const observer = mockObserverInstances[0];
		if (!observer) throw new Error("Observer should exist");
		const disconnectSpy = vi.spyOn(observer, "disconnect");

		action.destroy();

		expect(disconnectSpy).toHaveBeenCalled();
	});

	it("should recreate observer when options change via update", () => {
		const callback = vi.fn();

		const action = infiniteScroll(element, { callback, disabled: true });
		expect(mockObserverInstances.length).toBe(0);

		// Enable by updating disabled to false
		action.update({ callback, disabled: false });

		// A new observer should have been created
		expect(mockObserverInstances.length).toBe(1);
	});
});
