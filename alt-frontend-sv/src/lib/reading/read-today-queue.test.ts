import { describe, expect, it, vi } from "vitest";
import {
	type GetReadTodayQueueResult,
	QueueArticleOutcome,
	type QueueArticleResult,
	type ReadTodayQueueItem,
} from "#lib/connect/reading.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import {
	createReadTodayQueueStore,
	getLocalMidnight,
} from "./read-today-queue.svelte.js";

function makeQueueItem(
	articleId: string,
	overrides: Partial<ReadTodayQueueItem> = {},
): ReadTodayQueueItem {
	return {
		articleId,
		title: `Article ${articleId}`,
		url: `https://example.com/${articleId}`,
		sourceName: "Example Source",
		imageUrl: "",
		queuedAt: new Date("2026-11-18T10:00:00.000Z"),
		expiresAt: new Date("2026-11-20T10:00:00.000Z"),
		...overrides,
	};
}

describe("getLocalMidnight", () => {
	it("returns local 00:00:00.000 for the provided date", () => {
		const sample = new Date(2026, 10, 18, 17, 42, 13, 456);
		const midnight = getLocalMidnight(sample);
		expect(midnight.getFullYear()).toBe(2026);
		expect(midnight.getMonth()).toBe(10);
		expect(midnight.getDate()).toBe(18);
		expect(midnight.getHours()).toBe(0);
		expect(midnight.getMinutes()).toBe(0);
		expect(midnight.getSeconds()).toBe(0);
		expect(midnight.getMilliseconds()).toBe(0);
	});
});

describe("createReadTodayQueueStore", () => {
	it("loads queue items, cap, and readCountSince using device local midnight", async () => {
		let capturedSince: Date | undefined;
		const fixedNow = new Date(2026, 10, 18, 14, 30, 0, 0);
		const store = createReadTodayQueueStore({
			now: () => fixedNow.getTime(),
			getQueue: vi.fn(
				async (readSince: Date): Promise<GetReadTodayQueueResult> => {
					capturedSince = readSince;
					return {
						items: [makeQueueItem("art-1"), makeQueueItem("art-2")],
						cap: 7,
						readCountSince: 3,
					};
				},
			),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(),
		});

		await store.refresh();

		expect(capturedSince?.getTime()).toBe(getLocalMidnight(fixedNow).getTime());
		expect(store.items).toHaveLength(2);
		expect(store.cap).toBe(7);
		expect(store.readCountSince).toBe(3);
		expect(store.remainingSlots).toBe(5);
	});

	it("matches isQueued by articleId or normalized URL", async () => {
		const store = createReadTodayQueueStore({
			getQueue: vi.fn(async () => ({
				items: [
					makeQueueItem("art-1", {
						url: "https://example.com/posts/hello/",
					}),
				],
				cap: 7,
				readCountSince: 0,
			})),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(),
		});

		await store.refresh();

		expect(store.isQueued("art-1")).toBe(true);
		expect(store.isQueued("https://example.com/posts/hello")).toBe(true);
		expect(
			store.isQueued(
				"https://example.com/posts/hello/?utm_source=rss&utm_medium=feed",
			),
		).toBe(true);
		expect(store.isQueued("art-999")).toBe(false);
		expect(store.isQueued("https://example.com/other")).toBe(false);
		expect(store.isQueued("")).toBe(false);
		expect(store.isQueued(null)).toBe(false);
	});

	it("counts one pending, not-yet-sent queue action in remainingSlots", async () => {
		const store = createReadTodayQueueStore({
			getQueue: vi.fn(async () => ({
				items: Array.from({ length: 6 }, (_, i) => makeQueueItem(`art-${i}`)),
				cap: 7,
				readCountSince: 0,
			})),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(),
		});

		await store.refresh();
		expect(store.remainingSlots).toBe(1);

		store.setPendingQueueCount(1);
		expect(store.remainingSlots).toBe(0);

		store.setPendingQueueCount(0);
		expect(store.remainingSlots).toBe(1);
	});

	it("overlays recent local adds onto a lagging server refetch for ~15s", async () => {
		let nowMs = 1_000_000;
		const serverList: ReadTodayQueueItem[] = [makeQueueItem("art-1")];
		const getQueue = vi.fn(async () => ({
			items: [...serverList],
			cap: 7,
			readCountSince: 0,
		}));
		const queueItem = vi.fn(
			async (): Promise<QueueArticleResult> => ({
				outcome: QueueArticleOutcome.QUEUED,
				items: [makeQueueItem("art-1"), makeQueueItem("art-2")],
				cap: 7,
			}),
		);

		const store = createReadTodayQueueStore({
			now: () => nowMs,
			getQueue,
			queueItem,
			unqueueItem: vi.fn(),
		});

		await store.refresh();
		expect(store.items.map((i) => i.articleId)).toEqual(["art-1"]);

		await store.queue({
			articleId: "art-2",
			surface: ReadingSurface.SWIPE,
			title: "Article art-2",
			url: "https://example.com/art-2",
		});
		expect(store.isQueued("art-2")).toBe(true);

		// Lagging refetch within 15s still omits art-2 on the server, but local overlay preserves it.
		nowMs += 5_000;
		await store.refresh();
		expect(store.isQueued("art-2")).toBe(true);
		expect(store.items.map((i) => i.articleId)).toEqual(["art-1", "art-2"]);

		// Once 15s has passed, the server view is trusted again.
		nowMs += 11_000;
		await store.refresh();
		expect(store.isQueued("art-2")).toBe(false);
		expect(store.items.map((i) => i.articleId)).toEqual(["art-1"]);
	});

	it("overlays recent local removes onto a lagging server refetch for ~15s", async () => {
		let nowMs = 2_000_000;
		const store = createReadTodayQueueStore({
			now: () => nowMs,
			getQueue: vi.fn(async () => ({
				items: [makeQueueItem("art-1"), makeQueueItem("art-2")],
				cap: 7,
				readCountSince: 0,
			})),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(async () => {}),
		});

		await store.refresh();
		await store.unqueue("art-1");

		expect(store.isQueued("art-1")).toBe(false);
		expect(store.items.map((i) => i.articleId)).toEqual(["art-2"]);

		// Lagging refetch within 15s still returns art-1 from the server, but local overlay hides it.
		nowMs += 8_000;
		await store.refresh();
		expect(store.isQueued("art-1")).toBe(false);
		expect(store.items.map((i) => i.articleId)).toEqual(["art-2"]);

		// After 15s, the server view is trusted again.
		nowMs += 8_000;
		await store.refresh();
		expect(store.isQueued("art-1")).toBe(true);
	});

	it("rolls back optimistic removal if unqueue fails", async () => {
		const store = createReadTodayQueueStore({
			getQueue: vi.fn(async () => ({
				items: [makeQueueItem("art-1")],
				cap: 7,
				readCountSince: 0,
			})),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(async () => {
				throw new Error("network error");
			}),
		});

		await store.refresh();
		await expect(store.unqueue("art-1")).rejects.toThrow("network error");
		expect(store.isQueued("art-1")).toBe(true);
		expect(store.items).toHaveLength(1);
	});

	it("handles QUEUE_FULL without adding the item", async () => {
		const fullItems = Array.from({ length: 7 }, (_, i) =>
			makeQueueItem(`art-${i}`),
		);
		const store = createReadTodayQueueStore({
			getQueue: vi.fn(),
			queueItem: vi.fn(async () => ({
				outcome: QueueArticleOutcome.QUEUE_FULL,
				items: fullItems,
				cap: 7,
			})),
			unqueueItem: vi.fn(),
		});

		const result = await store.queue({
			articleId: "art-overflow",
			surface: ReadingSurface.SWIPE,
			title: "Overflow",
			url: "https://example.com/overflow",
		});

		expect(result.outcome).toBe(QueueArticleOutcome.QUEUE_FULL);
		expect(store.isQueued("art-overflow")).toBe(false);
		expect(store.remainingSlots).toBe(0);
	});

	it("drops articles returned in readArticleIds and keeps them suppressed on lagging refresh", async () => {
		let nowMs = 3_000_000;
		const store = createReadTodayQueueStore({
			now: () => nowMs,
			getQueue: vi.fn(async () => ({
				items: [makeQueueItem("art-1"), makeQueueItem("art-2")],
				cap: 7,
				readCountSince: 0,
			})),
			queueItem: vi.fn(),
			unqueueItem: vi.fn(),
		});

		await store.refresh();
		store.applyReadArticleIds(["art-1"]);

		expect(store.isQueued("art-1")).toBe(false);
		expect(store.items.map((i) => i.articleId)).toEqual(["art-2"]);

		nowMs += 5_000;
		await store.refresh();
		expect(store.isQueued("art-1")).toBe(false);
	});
});
