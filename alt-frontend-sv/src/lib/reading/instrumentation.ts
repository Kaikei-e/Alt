import { createKeepaliveTransport } from "#lib/connect/reading-transport.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	createDwellTracker,
	type VisibilitySource,
} from "#lib/reading/dwell-tracker.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import {
	createReadingEventQueue,
	type FlushOptions,
	type ReadingEventQueue,
	type ReadingEventQueueOptions,
	ReadingSurface,
} from "#lib/reading/reading-events.js";
import {
	createReadingSession,
	type ReadingSessionDwell,
} from "#lib/reading/reading-session.js";

export { ReadingSurface };

export interface LifecycleDocumentTarget {
	readonly visibilityState: DocumentVisibilityState;
	addEventListener(type: "visibilitychange", listener: () => void): void;
	removeEventListener(type: "visibilitychange", listener: () => void): void;
}

export interface LifecycleWindowTarget {
	addEventListener(type: "pagehide", listener: () => void): void;
	removeEventListener(type: "pagehide", listener: () => void): void;
}

export interface StartReadingSessionParams {
	articleId: string | null | undefined;
	surface: ReadingSurface;
	bodyLengthChars: number;
	dwell?: ReadingSessionDwell;
	visibility?: VisibilitySource | null;
	now?: () => number;
	onReadArticleIds?: (articleIds: string[]) => void;
}

export interface ManagedReadingSession {
	readonly isEnded: boolean;
	onScroll(ratio: number): void;
	markReachedEnd(): void;
	endAndFlush(): void;
}

export interface ReadingInstrumentation {
	readonly queue: ReadingEventQueue;
	recordSurfaceViewed(surface: ReadingSurface): void;
	recordArticleOpened(
		articleId: string | null | undefined,
		surface: ReadingSurface,
		onReadArticleIds?: (articleIds: string[]) => void,
	): void;
	startSession(params: StartReadingSessionParams): ManagedReadingSession | null;
	flush(options?: FlushOptions): Promise<void>;
	endOpenSessionsAndFlushKeepalive(): Promise<void>;
	registerLifecycle(targets?: {
		doc?: LifecycleDocumentTarget | null;
		win?: LifecycleWindowTarget | null;
	}): () => void;
}

export function measureDisplayedBodyChars(
	element: { readonly textContent: string | null } | null | undefined,
): number {
	return (element?.textContent ?? "").trim().length;
}

export function computeElementScrollRatio(container: {
	readonly scrollTop: number;
	readonly scrollHeight: number;
	readonly clientHeight: number;
}): number {
	if (container.scrollHeight <= 0) {
		return 0;
	}
	const maxScrollTop = container.scrollHeight - container.clientHeight;
	if (maxScrollTop <= 0) {
		return 1;
	}
	return Math.min(1, Math.max(0, container.scrollTop / maxScrollTop));
}

export interface WindowScrollMetrics {
	readonly scrollY: number;
	readonly innerHeight: number;
}

export interface DocumentScrollMetrics {
	readonly documentElement?: {
		readonly scrollTop: number;
		readonly scrollHeight: number;
		readonly clientHeight: number;
	} | null;
	readonly body?: {
		readonly scrollHeight: number;
	} | null;
}

export function computeWindowScrollRatio(
	win: WindowScrollMetrics | undefined = typeof window !== "undefined"
		? window
		: undefined,
	doc: DocumentScrollMetrics | undefined = typeof document !== "undefined"
		? document
		: undefined,
): number {
	if (!win || !doc) {
		return 0;
	}
	const docEl = doc.documentElement;
	const scrollTop = win.scrollY || docEl?.scrollTop || 0;
	const viewportHeight = win.innerHeight || docEl?.clientHeight || 0;
	const scrollHeight = Math.max(
		docEl?.scrollHeight ?? 0,
		doc.body?.scrollHeight ?? 0,
	);
	if (scrollHeight <= 0) {
		return 0;
	}
	const maxScrollTop = scrollHeight - viewportHeight;
	if (maxScrollTop <= 0) {
		return 1;
	}
	return Math.min(1, Math.max(0, scrollTop / maxScrollTop));
}

export function observeEndSentinel(
	sentinel: Element,
	root: Element | null,
	onReachedEnd: () => void,
	ObserverCtor:
		| typeof IntersectionObserver
		| undefined = typeof IntersectionObserver !== "undefined"
		? IntersectionObserver
		: undefined,
): () => void {
	if (!ObserverCtor) {
		return () => {};
	}
	let disconnected = false;
	const observer = new ObserverCtor(
		(entries) => {
			if (entries.some((entry) => entry.isIntersecting)) {
				onReachedEnd();
				if (!disconnected) {
					disconnected = true;
					observer.disconnect();
				}
			}
		},
		{ root },
	);
	observer.observe(sentinel);
	return () => {
		if (!disconnected) {
			disconnected = true;
			observer.disconnect();
		}
	};
}

export function createReadingInstrumentation(
	options: ReadingEventQueueOptions = {},
): ReadingInstrumentation {
	const articleReadListeners = new Map<
		string,
		(articleIds: string[]) => void
	>();
	const handleReadArticleIds = (ids: string[]) => {
		options.onReadArticleIds?.(ids);
		const invoked = new Set<(articleIds: string[]) => void>();
		for (const id of ids) {
			const cb = articleReadListeners.get(id);
			if (!cb) continue;
			articleReadListeners.delete(id);
			if (!invoked.has(cb)) {
				invoked.add(cb);
				cb(ids);
			}
		}
	};

	const baseOptions: ReadingEventQueueOptions =
		options.send || options.getTransport
			? options
			: {
					...options,
					getTransport: (keepalive) =>
						keepalive ? createKeepaliveTransport() : createClientTransport(),
				};
	const queueOptions: ReadingEventQueueOptions = {
		...baseOptions,
		onReadArticleIds: handleReadArticleIds,
	};
	const queue = createReadingEventQueue(queueOptions);
	const openSessionEnders = new Set<() => void>();

	let lifecycleRefs = 0;
	let detachLifecycle: (() => void) | null = null;

	const endOpenSessionsAndFlushKeepalive = (): Promise<void> => {
		if (openSessionEnders.size > 0) {
			const enders = [...openSessionEnders];
			openSessionEnders.clear();
			for (const end of enders) {
				end();
			}
		}
		return queue.flush({ keepalive: true });
	};

	return {
		get queue() {
			return queue;
		},
		recordSurfaceViewed(surface: ReadingSurface) {
			queue.enqueueSurfaceViewed(surface);
		},
		recordArticleOpened(
			articleId: string | null | undefined,
			surface: ReadingSurface,
			onReadArticleIds?: (articleIds: string[]) => void,
		) {
			const trimmed = articleId?.trim() ?? "";
			if (trimmed.length === 0) {
				return;
			}
			if (onReadArticleIds) {
				articleReadListeners.set(trimmed, onReadArticleIds);
			}
			queue.enqueueOpened(trimmed, surface);
		},
		startSession(
			params: StartReadingSessionParams,
		): ManagedReadingSession | null {
			const articleId = params.articleId?.trim() ?? "";
			if (articleId.length === 0) {
				return null;
			}
			if (params.onReadArticleIds) {
				articleReadListeners.set(articleId, params.onReadArticleIds);
			}
			const dwell =
				params.dwell ??
				(params.visibility !== undefined || params.now !== undefined
					? createDwellTracker({
							visibility: params.visibility,
							now: params.now,
						})
					: undefined);
			const session = createReadingSession({
				articleId,
				surface: params.surface,
				bodyLengthChars: params.bodyLengthChars,
				queue,
				dwell,
			});
			const endSession = () => {
				session.end();
			};
			openSessionEnders.add(endSession);

			return {
				get isEnded() {
					return session.isEnded;
				},
				onScroll(ratio: number) {
					session.onScroll(ratio);
				},
				markReachedEnd() {
					session.markReachedEnd();
				},
				endAndFlush() {
					if (!openSessionEnders.delete(endSession)) {
						return;
					}
					session.end();
					void queue.flush({ keepalive: false });
				},
			};
		},
		flush(flushOptions?: FlushOptions): Promise<void> {
			return queue.flush(flushOptions);
		},
		endOpenSessionsAndFlushKeepalive,
		registerLifecycle(targets) {
			const doc: LifecycleDocumentTarget | null =
				targets?.doc !== undefined
					? targets.doc
					: typeof document !== "undefined"
						? document
						: null;
			const win: LifecycleWindowTarget | null =
				targets?.win !== undefined
					? targets.win
					: typeof window !== "undefined"
						? window
						: null;

			lifecycleRefs += 1;
			if (lifecycleRefs === 1) {
				const onVisibilityChange = () => {
					if (doc?.visibilityState === "hidden") {
						void endOpenSessionsAndFlushKeepalive();
					}
				};
				const onPageHide = () => {
					void endOpenSessionsAndFlushKeepalive();
				};
				doc?.addEventListener("visibilitychange", onVisibilityChange);
				win?.addEventListener("pagehide", onPageHide);
				detachLifecycle = () => {
					doc?.removeEventListener("visibilitychange", onVisibilityChange);
					win?.removeEventListener("pagehide", onPageHide);
				};
			}

			let cleaned = false;
			return () => {
				if (cleaned) {
					return;
				}
				cleaned = true;
				lifecycleRefs = Math.max(0, lifecycleRefs - 1);
				if (lifecycleRefs === 0 && detachLifecycle) {
					detachLifecycle();
					detachLifecycle = null;
				}
			};
		},
	};
}

const defaultInstrumentation = createReadingInstrumentation({
	onReadArticleIds: (ids) => {
		readTodayQueue.applyReadArticleIds(ids);
	},
});

export function recordSurfaceViewed(surface: ReadingSurface): void {
	defaultInstrumentation.recordSurfaceViewed(surface);
}

export function recordArticleOpened(
	articleId: string | null | undefined,
	surface: ReadingSurface,
	onReadArticleIds?: (articleIds: string[]) => void,
): void {
	defaultInstrumentation.recordArticleOpened(
		articleId,
		surface,
		onReadArticleIds,
	);
}

export function flushReadingEvents(options?: FlushOptions): Promise<void> {
	return defaultInstrumentation.flush(options);
}

export function registerReadingLifecycle(): () => void {
	return defaultInstrumentation.registerLifecycle();
}

export interface ElementScrollTarget {
	readonly scrollTop: number;
	readonly scrollHeight: number;
	readonly clientHeight: number;
	addEventListener(
		type: "scroll",
		listener: () => void,
		options?: { passive?: boolean },
	): void;
	removeEventListener(type: "scroll", listener: () => void): void;
}

export interface WindowScrollTarget extends WindowScrollMetrics {
	addEventListener(
		type: "scroll",
		listener: () => void,
		options?: { passive?: boolean },
	): void;
	removeEventListener(type: "scroll", listener: () => void): void;
}

interface ReadingSessionAttachmentConfig {
	articleId: string | null | undefined;
	surface: ReadingSurface;
	bodyElement: { readonly textContent: string | null } | null;
	endSentinel: Element | null;
	observerRoot: Element | null;
	getScrollRatio: () => number;
	addScrollListener: (onScroll: () => void) => () => void;
	instrumentation?: ReadingInstrumentation;
	doc?: LifecycleDocumentTarget | null;
	observerCtor?: typeof IntersectionObserver;
	now?: () => number;
	onReadArticleIds?: (articleIds: string[]) => void;
}

function attachReadingSession(
	config: ReadingSessionAttachmentConfig,
): (() => void) | undefined {
	const articleId = config.articleId?.trim() ?? "";
	if (articleId.length === 0 || !config.bodyElement || !config.endSentinel) {
		return undefined;
	}

	const instrumentation = config.instrumentation ?? defaultInstrumentation;
	const doc: LifecycleDocumentTarget | null =
		config.doc !== undefined
			? config.doc
			: typeof document !== "undefined"
				? document
				: null;
	const bodyElement = config.bodyElement;
	const endSentinel = config.endSentinel;

	let currentSession: ManagedReadingSession | null = null;
	let stopObserver: (() => void) | null = null;
	let cleaned = false;

	const stopCurrentObserver = () => {
		if (stopObserver) {
			stopObserver();
			stopObserver = null;
		}
	};

	const beginSession = () => {
		if (cleaned) {
			return;
		}
		stopCurrentObserver();
		const session = instrumentation.startSession({
			articleId,
			surface: config.surface,
			bodyLengthChars: measureDisplayedBodyChars(bodyElement),
			visibility: doc,
			now: config.now,
			onReadArticleIds: config.onReadArticleIds,
		});
		if (!session) {
			return;
		}
		currentSession = session;
		session.onScroll(config.getScrollRatio());
		stopObserver = observeEndSentinel(
			endSentinel,
			config.observerRoot,
			() => {
				session.markReachedEnd();
			},
			config.observerCtor,
		);
	};

	const onScroll = () => {
		if (!currentSession || currentSession.isEnded) {
			return;
		}
		currentSession.onScroll(config.getScrollRatio());
	};

	const onVisibilityChange = () => {
		if (cleaned || !doc) {
			return;
		}
		if (doc.visibilityState === "hidden") {
			stopCurrentObserver();
			return;
		}
		if (
			doc.visibilityState === "visible" &&
			(!currentSession || currentSession.isEnded)
		) {
			beginSession();
		}
	};

	const stopScrollListener = config.addScrollListener(onScroll);
	doc?.addEventListener("visibilitychange", onVisibilityChange);

	if (doc?.visibilityState !== "hidden") {
		beginSession();
	}

	return () => {
		if (cleaned) {
			return;
		}
		cleaned = true;
		doc?.removeEventListener("visibilitychange", onVisibilityChange);
		stopScrollListener();
		stopCurrentObserver();
		currentSession?.endAndFlush();
		currentSession = null;
	};
}

export interface ElementReadingSessionAttachment {
	articleId: string | null | undefined;
	surface: ReadingSurface;
	bodyElement: { readonly textContent: string | null } | null;
	scrollContainer: ElementScrollTarget | null;
	endSentinel: Element | null;
	instrumentation?: ReadingInstrumentation;
	doc?: LifecycleDocumentTarget | null;
	observerCtor?: typeof IntersectionObserver;
	now?: () => number;
	onReadArticleIds?: (articleIds: string[]) => void;
}

export function attachElementReadingSession(
	params: ElementReadingSessionAttachment,
): (() => void) | undefined {
	const container = params.scrollContainer;
	if (!container) {
		return undefined;
	}
	return attachReadingSession({
		articleId: params.articleId,
		surface: params.surface,
		bodyElement: params.bodyElement,
		endSentinel: params.endSentinel,
		observerRoot: container as unknown as Element,
		getScrollRatio: () => computeElementScrollRatio(container),
		addScrollListener: (onScroll) => {
			container.addEventListener("scroll", onScroll, { passive: true });
			return () => container.removeEventListener("scroll", onScroll);
		},
		instrumentation: params.instrumentation,
		doc: params.doc,
		observerCtor: params.observerCtor,
		now: params.now,
		onReadArticleIds: params.onReadArticleIds,
	});
}

export interface WindowReadingSessionAttachment {
	articleId: string | null | undefined;
	surface: ReadingSurface;
	bodyElement: { readonly textContent: string | null } | null;
	endSentinel: Element | null;
	instrumentation?: ReadingInstrumentation;
	win?: WindowScrollTarget | null;
	doc?: (LifecycleDocumentTarget & DocumentScrollMetrics) | null;
	observerCtor?: typeof IntersectionObserver;
	now?: () => number;
	onReadArticleIds?: (articleIds: string[]) => void;
}

export function attachWindowReadingSession(
	params: WindowReadingSessionAttachment,
): (() => void) | undefined {
	const win: WindowScrollTarget | null =
		params.win !== undefined
			? params.win
			: typeof window !== "undefined"
				? window
				: null;
	const doc: (LifecycleDocumentTarget & DocumentScrollMetrics) | null =
		params.doc !== undefined
			? params.doc
			: typeof document !== "undefined"
				? document
				: null;
	if (!win) {
		return undefined;
	}
	return attachReadingSession({
		articleId: params.articleId,
		surface: params.surface,
		bodyElement: params.bodyElement,
		endSentinel: params.endSentinel,
		observerRoot: null,
		getScrollRatio: () => computeWindowScrollRatio(win, doc ?? undefined),
		addScrollListener: (onScroll) => {
			win.addEventListener("scroll", onScroll, { passive: true });
			return () => win.removeEventListener("scroll", onScroll);
		},
		instrumentation: params.instrumentation,
		doc,
		observerCtor: params.observerCtor,
		now: params.now,
		onReadArticleIds: params.onReadArticleIds,
	});
}
