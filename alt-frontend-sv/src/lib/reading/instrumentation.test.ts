import { describe, expect, it, vi } from "vitest";
import {
	type ReadingEvent,
	ReadingSurface,
} from "#lib/gen/alt/reading/v1/reading_pb.js";
import {
	attachElementReadingSession,
	attachWindowReadingSession,
	computeElementScrollRatio,
	computeWindowScrollRatio,
	createReadingInstrumentation,
	measureDisplayedBodyChars,
	observeEndSentinel,
} from "./instrumentation";

type SendFn = (
	events: ReadingEvent[],
	options: { keepalive: boolean },
) => Promise<void>;

function createFakeTargets(
	initialVisibility: DocumentVisibilityState = "visible",
) {
	let visibilityState = initialVisibility;
	const docListeners = new Set<() => void>();
	const winListeners = new Set<() => void>();

	return {
		doc: {
			get visibilityState() {
				return visibilityState;
			},
			addEventListener: vi.fn((_type: "visibilitychange", fn: () => void) => {
				docListeners.add(fn);
			}),
			removeEventListener: vi.fn(
				(_type: "visibilitychange", fn: () => void) => {
					docListeners.delete(fn);
				},
			),
		},
		win: {
			addEventListener: vi.fn((_type: "pagehide", fn: () => void) => {
				winListeners.add(fn);
			}),
			removeEventListener: vi.fn((_type: "pagehide", fn: () => void) => {
				winListeners.delete(fn);
			}),
		},
		setVisibility(next: DocumentVisibilityState) {
			visibilityState = next;
			for (const fn of [...docListeners]) {
				fn();
			}
		},
		firePageHide() {
			for (const fn of [...winListeners]) {
				fn();
			}
		},
	};
}

describe("reading-instrumentation", () => {
	it("enqueues surface_viewed without flushing immediately", () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });

		instrumentation.recordSurfaceViewed(ReadingSurface.GALLERY);

		expect(send).not.toHaveBeenCalled();
		expect(instrumentation.queue.size).toBe(1);
	});

	it("returns null and records nothing when articleId is missing or blank", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });

		expect(
			instrumentation.startSession({
				articleId: null,
				surface: ReadingSurface.GALLERY,
				bodyLengthChars: 500,
			}),
		).toBeNull();
		expect(
			instrumentation.startSession({
				articleId: "   ",
				surface: ReadingSurface.SWIPE,
				bodyLengthChars: 500,
			}),
		).toBeNull();

		await instrumentation.flush();
		expect(send).not.toHaveBeenCalled();
	});

	it("flushes without keepalive when a session ends and is idempotent on repeat calls", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });

		instrumentation.recordSurfaceViewed(ReadingSurface.GALLERY);
		const session = instrumentation.startSession({
			articleId: "art-1",
			surface: ReadingSurface.GALLERY,
			bodyLengthChars: 1234,
			dwell: { stop: () => 4200 },
		});
		expect(session).not.toBeNull();

		session?.onScroll(0.8);
		session?.markReachedEnd();
		expect(send).not.toHaveBeenCalled();

		session?.endAndFlush();
		session?.endAndFlush();
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(1);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: false });
		const events = send.mock.calls[0]?.[0] ?? [];
		expect(events.map((e) => e.event.case)).toEqual([
			"surfaceViewed",
			"reachedEnd",
			"sessionEnded",
		]);
		expect(events[2]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-1",
				surface: ReadingSurface.GALLERY,
				activeDwellMs: 4200n,
				maxScrollRatio: 0.8,
				bodyLengthChars: 1234,
				reachedEnd: true,
			},
		});
	});

	it("registers lifecycle listeners once via refcounting and ends open sessions before keepalive flush on hidden and pagehide", async () => {
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });
		const targets = createFakeTargets("visible");

		const cleanupA = instrumentation.registerLifecycle(targets);
		const cleanupB = instrumentation.registerLifecycle(targets);

		expect(targets.doc.addEventListener).toHaveBeenCalledTimes(1);
		expect(targets.win.addEventListener).toHaveBeenCalledTimes(1);

		const session = instrumentation.startSession({
			articleId: "art-hidden",
			surface: ReadingSurface.SWIPE,
			bodyLengthChars: 800,
			dwell: { stop: () => 2500 },
		});

		targets.setVisibility("visible");
		expect(send).not.toHaveBeenCalled();

		targets.setVisibility("hidden");
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(1);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: true });
		expect(send.mock.calls[0]?.[0]?.[0]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-hidden",
				surface: ReadingSurface.SWIPE,
				activeDwellMs: 2500n,
			},
		});

		// Subsequent component teardown or pagehide does not re-emit the ended session.
		session?.endAndFlush();
		targets.firePageHide();
		await Promise.resolve();
		expect(send).toHaveBeenCalledTimes(1);

		// A standalone surface view with no open session still flushes on pagehide.
		instrumentation.recordSurfaceViewed(ReadingSurface.ARTICLE_PAGE);
		targets.firePageHide();
		await Promise.resolve();
		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[1]?.[1]).toEqual({ keepalive: true });

		cleanupA();
		expect(targets.doc.removeEventListener).not.toHaveBeenCalled();
		cleanupB();
		expect(targets.doc.removeEventListener).toHaveBeenCalledTimes(1);
		expect(targets.win.removeEventListener).toHaveBeenCalledTimes(1);
	});

	it("computes element and window scroll ratios and displayed body character length", () => {
		expect(
			computeElementScrollRatio({
				scrollTop: 300,
				scrollHeight: 1000,
				clientHeight: 400,
			}),
		).toBeCloseTo(0.5);
		expect(
			computeElementScrollRatio({
				scrollTop: 0,
				scrollHeight: 300,
				clientHeight: 500,
			}),
		).toBe(1);
		expect(
			computeElementScrollRatio({
				scrollTop: 0,
				scrollHeight: 0,
				clientHeight: 0,
			}),
		).toBe(0);

		expect(
			computeWindowScrollRatio(
				{ scrollY: 200, innerHeight: 600 } as unknown as Window,
				{
					documentElement: {
						scrollTop: 200,
						scrollHeight: 1000,
						clientHeight: 600,
					},
					body: { scrollHeight: 900 },
				} as unknown as Document,
			),
		).toBeCloseTo(0.5);

		expect(
			measureDisplayedBodyChars({ textContent: "\n  Hello reader!  \n" }),
		).toBe(13);
		expect(measureDisplayedBodyChars(null)).toBe(0);
	});

	it("marks reached-end once via IntersectionObserver and disconnects", () => {
		const captured: { callback?: IntersectionObserverCallback } = {};
		let observedRoot: Element | Document | null | undefined;
		const observe = vi.fn();
		const disconnect = vi.fn();

		class FakeIntersectionObserver {
			constructor(
				callback: IntersectionObserverCallback,
				options?: IntersectionObserverInit,
			) {
				captured.callback = callback;
				observedRoot = options?.root;
			}
			observe = observe;
			disconnect = disconnect;
			unobserve = vi.fn();
			takeRecords = vi.fn(() => []);
			root = null;
			rootMargin = "";
			thresholds = [];
		}

		const sentinel = {} as Element;
		const root = {} as Element;
		const onReachedEnd = vi.fn();

		const cleanup = observeEndSentinel(
			sentinel,
			root,
			onReachedEnd,
			FakeIntersectionObserver as unknown as typeof IntersectionObserver,
		);

		expect(observedRoot).toBe(root);
		expect(observe).toHaveBeenCalledWith(sentinel);

		captured.callback?.(
			[{ isIntersecting: false } as IntersectionObserverEntry],
			{} as IntersectionObserver,
		);
		expect(onReachedEnd).not.toHaveBeenCalled();

		captured.callback?.(
			[{ isIntersecting: true } as IntersectionObserverEntry],
			{} as IntersectionObserver,
		);
		expect(onReachedEnd).toHaveBeenCalledTimes(1);
		expect(disconnect).toHaveBeenCalledTimes(1);

		cleanup();
		expect(disconnect).toHaveBeenCalledTimes(1);
	});

	it("resumes element reading session when document becomes visible again with new dwell, seeded scroll ratio, and re-observed sentinel", async () => {
		let now = 1000;
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });
		const targets = createFakeTargets("visible");
		const stopLifecycle = instrumentation.registerLifecycle(targets);

		const scrollListeners = new Set<() => void>();
		const scrollContainer = {
			scrollTop: 0,
			scrollHeight: 1000,
			clientHeight: 400,
			addEventListener: vi.fn((_type: "scroll", fn: () => void) => {
				scrollListeners.add(fn);
			}),
			removeEventListener: vi.fn((_type: "scroll", fn: () => void) => {
				scrollListeners.delete(fn);
			}),
		};

		const observerCallbacks: IntersectionObserverCallback[] = [];
		const observerDisconnects: ReturnType<typeof vi.fn>[] = [];
		class FakeIntersectionObserver {
			callback: IntersectionObserverCallback;
			constructor(callback: IntersectionObserverCallback) {
				this.callback = callback;
				observerCallbacks.push(callback);
				observerDisconnects.push(this.disconnect);
			}
			observe = vi.fn();
			disconnect = vi.fn();
			unobserve = vi.fn();
			takeRecords = vi.fn(() => []);
			root = null;
			rootMargin = "";
			thresholds = [];
		}

		const cleanup = attachElementReadingSession({
			articleId: "art-resume-el",
			surface: ReadingSurface.GALLERY,
			bodyElement: { textContent: "Hello world body text" },
			scrollContainer,
			endSentinel: {} as Element,
			instrumentation,
			doc: targets.doc,
			observerCtor:
				FakeIntersectionObserver as unknown as typeof IntersectionObserver,
			now: () => now,
		});
		expect(cleanup).toBeDefined();

		now += 3000;
		scrollContainer.scrollTop = 300; // ratio 0.5
		for (const fn of scrollListeners) fn();

		targets.setVisibility("hidden");
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(1);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: true });
		expect(send.mock.calls[0]?.[0]?.[0]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-resume-el",
				surface: ReadingSurface.GALLERY,
				activeDwellMs: 3000n,
				maxScrollRatio: 0.5,
				reachedEnd: false,
			},
		});

		// Time passes while page is hidden.
		now += 20000;
		targets.setVisibility("visible");
		expect(observerCallbacks).toHaveLength(2);

		// Repeated visible event while session is active does not fragment.
		targets.setVisibility("visible");
		expect(observerCallbacks).toHaveLength(2);

		now += 1500;
		scrollContainer.scrollTop = 480; // ratio 0.8
		for (const fn of scrollListeners) fn();
		observerCallbacks[1]?.(
			[{ isIntersecting: true } as IntersectionObserverEntry],
			{} as IntersectionObserver,
		);

		cleanup?.();
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[1]?.[1]).toEqual({ keepalive: false });
		const secondBatch = send.mock.calls[1]?.[0] ?? [];
		expect(secondBatch.map((e) => e.event.case)).toEqual([
			"reachedEnd",
			"sessionEnded",
		]);
		expect(secondBatch[1]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-resume-el",
				surface: ReadingSurface.GALLERY,
				activeDwellMs: 1500n,
				maxScrollRatio: 0.8,
				reachedEnd: true,
			},
		});

		stopLifecycle();
	});

	it("resumes window reading session when document becomes visible again and seeds scroll ratio from current position", async () => {
		let now = 5000;
		const send = vi.fn<SendFn>().mockResolvedValue(undefined);
		const instrumentation = createReadingInstrumentation({ send });
		const targets = createFakeTargets("visible");
		const stopLifecycle = instrumentation.registerLifecycle(targets);

		const winListeners = new Set<() => void>();
		const fakeWin = {
			scrollY: 0,
			innerHeight: 500,
			addEventListener: vi.fn((type: string, fn: () => void) => {
				if (type === "scroll") winListeners.add(fn);
			}),
			removeEventListener: vi.fn((type: string, fn: () => void) => {
				if (type === "scroll") winListeners.delete(fn);
			}),
		};
		const fakeDoc = Object.assign(targets.doc, {
			documentElement: {
				scrollTop: 0,
				scrollHeight: 1500,
				clientHeight: 500,
			},
			body: { scrollHeight: 1500 },
		});

		const observerCallbacks: IntersectionObserverCallback[] = [];
		class FakeIntersectionObserver {
			constructor(callback: IntersectionObserverCallback) {
				observerCallbacks.push(callback);
			}
			observe = vi.fn();
			disconnect = vi.fn();
			unobserve = vi.fn();
			takeRecords = vi.fn(() => []);
			root = null;
			rootMargin = "";
			thresholds = [];
		}

		const cleanup = attachWindowReadingSession({
			articleId: "art-resume-win",
			surface: ReadingSurface.ARTICLE_PAGE,
			bodyElement: { textContent: "Window body content" },
			endSentinel: {} as Element,
			instrumentation,
			win: fakeWin,
			doc: fakeDoc,
			observerCtor:
				FakeIntersectionObserver as unknown as typeof IntersectionObserver,
			now: () => now,
		});
		expect(cleanup).toBeDefined();

		now += 2000;
		fakeWin.scrollY = 500; // ratio 0.5
		for (const fn of winListeners) fn();

		targets.setVisibility("hidden");
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(1);
		expect(send.mock.calls[0]?.[1]).toEqual({ keepalive: true });
		expect(send.mock.calls[0]?.[0]?.[0]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-resume-win",
				activeDwellMs: 2000n,
				maxScrollRatio: 0.5,
			},
		});

		now += 10000;
		targets.setVisibility("visible");
		now += 1200;
		// Even without another scroll event, maxScrollRatio is seeded at 0.5.
		cleanup?.();
		await Promise.resolve();

		expect(send).toHaveBeenCalledTimes(2);
		expect(send.mock.calls[1]?.[1]).toEqual({ keepalive: false });
		expect(send.mock.calls[1]?.[0]?.[0]?.event).toMatchObject({
			case: "sessionEnded",
			value: {
				articleId: "art-resume-win",
				surface: ReadingSurface.ARTICLE_PAGE,
				activeDwellMs: 1200n,
				maxScrollRatio: 0.5,
				reachedEnd: false,
			},
		});

		stopLifecycle();
	});
});
