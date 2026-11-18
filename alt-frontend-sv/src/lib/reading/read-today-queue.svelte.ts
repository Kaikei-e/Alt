import {
	type GetReadTodayQueueResult,
	getReadTodayQueue,
	QueueArticleOutcome,
	type QueueArticleResult,
	queueArticle,
	type ReadingSurface,
	type ReadTodayQueueItem,
	unqueueArticle,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { normalizeUrl } from "#lib/utils/feed.js";

const DEFAULT_QUEUE_CAP = 7;
// Read-model projections can lag briefly behind writes; overlay recent local
// mutations so a refetch neither drops a newly queued item nor resurrects a
// removed one.
const LOCAL_OVERLAY_TTL_MS = 15_000;
const FORTY_EIGHT_HOURS_MS = 48 * 60 * 60 * 1000;

export interface QueueItemInput {
	articleId: string;
	surface: ReadingSurface;
	title?: string | null;
	url?: string | null;
	sourceName?: string | null;
	imageUrl?: string | null;
}

export interface ReadTodayQueueStoreDeps {
	now?: () => number;
	getQueue?: (readSince: Date) => Promise<GetReadTodayQueueResult>;
	queueItem?: (input: QueueItemInput) => Promise<QueueArticleResult>;
	unqueueItem?: (articleId: string) => Promise<void>;
}

interface OverlayAdd {
	item: ReadTodayQueueItem;
	untilMs: number;
}

export function getLocalMidnight(date: Date = new Date()): Date {
	return new Date(
		date.getFullYear(),
		date.getMonth(),
		date.getDate(),
		0,
		0,
		0,
		0,
	);
}

function mergeOverlay(
	serverItems: ReadTodayQueueItem[],
	localAdds: Map<string, OverlayAdd>,
	localRemoves: Map<string, number>,
	nowMs: number,
): ReadTodayQueueItem[] {
	for (const [id, entry] of localAdds) {
		if (entry.untilMs <= nowMs) {
			localAdds.delete(id);
		}
	}
	for (const [id, untilMs] of localRemoves) {
		if (untilMs <= nowMs) {
			localRemoves.delete(id);
		}
	}

	const result: ReadTodayQueueItem[] = [];
	const seenIds = new Set<string>();

	for (const item of serverItems) {
		if (localRemoves.has(item.articleId)) continue;
		result.push(item);
		seenIds.add(item.articleId);
	}

	for (const [id, entry] of localAdds) {
		if (localRemoves.has(id) || seenIds.has(id)) continue;
		result.push(entry.item);
		seenIds.add(id);
	}

	return result;
}

export function createReadTodayQueueStore(deps: ReadTodayQueueStoreDeps = {}) {
	const now = deps.now ?? (() => Date.now());
	const fetchQueue =
		deps.getQueue ??
		((readSince: Date) =>
			getReadTodayQueue(createClientTransport(), readSince));
	const sendQueue =
		deps.queueItem ??
		((input: QueueItemInput) =>
			queueArticle(createClientTransport(), {
				articleId: input.articleId,
				surface: input.surface,
			}));
	const sendUnqueue =
		deps.unqueueItem ??
		((articleId: string) =>
			unqueueArticle(createClientTransport(), { articleId }));

	let serverItems = $state<ReadTodayQueueItem[]>([]);
	let items = $state<ReadTodayQueueItem[]>([]);
	let cap = $state(DEFAULT_QUEUE_CAP);
	let readCountSince = $state(0);
	let pendingQueueCount = $state(0);
	let isLoading = $state(false);
	let hasLoaded = $state(false);
	let error = $state<string | null>(null);

	const localAdds = new Map<string, OverlayAdd>();
	const localRemoves = new Map<string, number>();
	let refreshToken = 0;

	function recompute() {
		items = mergeOverlay(serverItems, localAdds, localRemoves, now());
	}

	const remainingSlots = $derived(
		Math.max(0, cap - items.length - pendingQueueCount),
	);

	function isQueued(urlOrArticleId: string | null | undefined): boolean {
		const target = urlOrArticleId?.trim();
		if (!target) return false;
		const normalizedTarget = normalizeUrl(target);
		for (const item of items) {
			if (item.articleId === target) return true;
			if (!item.url) continue;
			if (item.url === target || normalizeUrl(item.url) === normalizedTarget) {
				return true;
			}
		}
		return false;
	}

	function setPendingQueueCount(count: number): void {
		pendingQueueCount = Math.max(0, Math.trunc(count));
	}

	async function refresh(): Promise<void> {
		const token = ++refreshToken;
		isLoading = true;
		error = null;
		try {
			const result = await fetchQueue(getLocalMidnight(new Date(now())));
			if (token !== refreshToken) return;
			serverItems = result.items;
			if (result.cap > 0) {
				cap = result.cap;
			}
			readCountSince = result.readCountSince;
			hasLoaded = true;
			recompute();
		} catch (err) {
			if (token !== refreshToken) return;
			error =
				err instanceof Error ? err.message : "Failed to load Read today queue";
			throw err;
		} finally {
			if (token === refreshToken) {
				isLoading = false;
			}
		}
	}

	async function queue(input: QueueItemInput): Promise<QueueArticleResult> {
		const articleId = input.articleId.trim();
		if (!articleId) {
			throw new Error("articleId is required to queue an article");
		}
		const result = await sendQueue({ ...input, articleId });
		if (result.cap > 0) {
			cap = result.cap;
		}
		serverItems = result.items;

		if (
			result.outcome === QueueArticleOutcome.QUEUED ||
			result.outcome === QueueArticleOutcome.ALREADY_QUEUED
		) {
			const nowMs = now();
			localRemoves.delete(articleId);
			const fromServer = result.items.find((i) => i.articleId === articleId);
			const fallbackItem: ReadTodayQueueItem = {
				articleId,
				title: input.title?.trim() || "Untitled",
				url: input.url?.trim() || "",
				sourceName: input.sourceName?.trim() || "",
				imageUrl: input.imageUrl?.trim() || "",
				queuedAt: new Date(nowMs),
				expiresAt: new Date(nowMs + FORTY_EIGHT_HOURS_MS),
			};
			localAdds.set(articleId, {
				item: fromServer ?? fallbackItem,
				untilMs: nowMs + LOCAL_OVERLAY_TTL_MS,
			});
			hasLoaded = true;
		}

		recompute();
		return result;
	}

	async function unqueue(articleId: string): Promise<void> {
		const targetId = articleId.trim();
		if (!targetId) return;

		const nowMs = now();
		const previousAdd = localAdds.get(targetId);
		const previousRemove = localRemoves.get(targetId);

		localAdds.delete(targetId);
		localRemoves.set(targetId, nowMs + LOCAL_OVERLAY_TTL_MS);
		recompute();

		try {
			await sendUnqueue(targetId);
		} catch (err) {
			localRemoves.delete(targetId);
			if (previousRemove !== undefined && previousRemove > now()) {
				localRemoves.set(targetId, previousRemove);
			}
			if (previousAdd !== undefined) {
				localAdds.set(targetId, previousAdd);
			}
			recompute();
			throw err;
		}
	}

	function applyReadArticleIds(articleIds: readonly string[]): void {
		if (!articleIds || articleIds.length === 0) return;
		const nowMs = now();
		const idSet = new Set<string>();
		for (const rawId of articleIds) {
			const id = rawId.trim();
			if (!id) continue;
			idSet.add(id);
			localAdds.delete(id);
			localRemoves.set(id, nowMs + LOCAL_OVERLAY_TTL_MS);
		}
		if (idSet.size === 0) return;
		serverItems = serverItems.filter((item) => !idSet.has(item.articleId));
		recompute();
	}

	function reset(): void {
		refreshToken++;
		serverItems = [];
		items = [];
		cap = DEFAULT_QUEUE_CAP;
		readCountSince = 0;
		pendingQueueCount = 0;
		isLoading = false;
		hasLoaded = false;
		error = null;
		localAdds.clear();
		localRemoves.clear();
	}

	return {
		get items() {
			return items;
		},
		get cap() {
			return cap;
		},
		get readCountSince() {
			return readCountSince;
		},
		get remainingSlots() {
			return remainingSlots;
		},
		get isLoading() {
			return isLoading;
		},
		get hasLoaded() {
			return hasLoaded;
		},
		get error() {
			return error;
		},
		isQueued,
		setPendingQueueCount,
		refresh,
		queue,
		unqueue,
		applyReadArticleIds,
		reset,
	};
}

export const readTodayQueue = createReadTodayQueueStore();
