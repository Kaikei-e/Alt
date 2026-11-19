import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { getReadTodayQueue } from "#lib/connect/reading.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import type { RenderFeed } from "#lib/schema/feed.js";
import FeedGrid from "./FeedGrid.svelte";
import type { FeedGridApi } from "./feed-grid-types.js";

vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

function makeFeed(
	index: number,
	overrides: Partial<RenderFeed> = {},
): RenderFeed {
	const id = `01938e82-7c00-7a7b-9b10-${String(index).padStart(12, "0")}`;
	return {
		id: `feed-${index}`,
		articleId: id,
		title: `Dispatch ${index}`,
		description: `Description ${index}`,
		link: `https://example.com/articles/${index}`,
		normalizedUrl: `https://example.com/articles/${index}`,
		published: "2026-11-18T12:00:00Z",
		created_at: `2026-11-18T12:${String(index).padStart(2, "0")}:00Z`,
		author: "Alt",
		publishedAtFormatted: "Nov 18, 2026",
		mergedTagsLabel: "Tech",
		excerpt: `Excerpt ${index}`,
		isRead: false,
		...overrides,
	};
}

async function settle(ms = 200) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("FeedGrid pagination under hidden rows", () => {
	beforeEach(() => {
		readTodayQueue.reset();
		vi.mocked(getReadTodayQueue).mockReset();
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [],
			cap: 7,
			readCountSince: 0,
		});
	});

	it("loads more when queued rows reduce visible count below a page while hasNextPage is true", async () => {
		const page1 = Array.from({ length: 20 }, (_, i) => makeFeed(i + 1));
		const page2 = Array.from({ length: 5 }, (_, i) => makeFeed(21 + i));

		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: page1.slice(0, 5).map((f) => ({
				articleId: f.articleId ?? "",
				title: f.title,
				url: f.normalizedUrl,
				sourceName: "Alt",
				imageUrl: "",
				queuedAt: new Date(),
				expiresAt: new Date(Date.now() + 3600_000),
			})),
			cap: 7,
			readCountSince: 0,
		});

		const fetchFn = vi
			.fn()
			.mockResolvedValueOnce({
				data: page1,
				next_cursor: "cursor-2",
				has_more: true,
			})
			.mockResolvedValueOnce({
				data: page2,
				next_cursor: "",
				has_more: false,
			});

		render(FeedGrid, {
			props: {
				unreadOnly: true,
				excludeQueued: true,
				fetchFn,
				onSelectFeed: vi.fn(),
			},
		});

		await vi.waitFor(() => expect(fetchFn).toHaveBeenCalledTimes(2));
		await expect
			.element(page.getByText("Dispatch 21", { exact: true }))
			.toBeInTheDocument();
		expect(page.getByText("Dispatch 1", { exact: true }).query()).toBeNull();
	});

	it("recovers instead of stalling on empty state when all visible rows are marked read while hasNextPage is true", async () => {
		await page.viewport(800, 600);
		const page1 = Array.from({ length: 20 }, (_, i) => makeFeed(i + 1));
		const page2 = [makeFeed(21), makeFeed(22)];

		const fetchFn = vi
			.fn()
			.mockResolvedValueOnce({
				data: page1,
				next_cursor: "cursor-2",
				has_more: true,
			})
			.mockResolvedValueOnce({
				data: page2,
				next_cursor: "",
				has_more: false,
			});

		let gridApi: FeedGridApi | null = null;
		const onReady = (api: FeedGridApi) => {
			gridApi = api;
		};
		render(FeedGrid, {
			props: {
				unreadOnly: true,
				excludeQueued: true,
				fetchFn,
				onSelectFeed: vi.fn(),
				onReady,
			},
		});

		await vi.waitFor(() => expect(gridApi).not.toBeNull());
		await expect
			.element(page.getByText("Dispatch 1", { exact: true }))
			.toBeInTheDocument();
		expect(fetchFn).toHaveBeenCalledTimes(1);

		// Mark all 20 rows from page 1 as read -> visible rows drop to 0 while hasNextPage is true
		const api = gridApi as FeedGridApi | null;
		api?.markFeedReadByArticleIds(page1.map((f) => f.articleId ?? ""));

		await vi.waitFor(() => expect(fetchFn).toHaveBeenCalledTimes(2));
		await expect
			.element(page.getByText("Dispatch 21", { exact: true }))
			.toBeInTheDocument();
	});

	it("stops auto-loading when a page returns no rows even if has_more is true", async () => {
		const page1 = Array.from({ length: 5 }, (_, i) => makeFeed(i + 1));
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: page1.map((f) => ({
				articleId: f.articleId ?? "",
				title: f.title,
				url: f.normalizedUrl,
				sourceName: "Alt",
				imageUrl: "",
				queuedAt: new Date(),
				expiresAt: new Date(Date.now() + 3600_000),
			})),
			cap: 7,
			readCountSince: 0,
		});

		const fetchFn = vi
			.fn()
			.mockResolvedValueOnce({
				data: page1,
				next_cursor: "cursor-2",
				has_more: true,
			})
			.mockResolvedValue({
				data: [],
				next_cursor: "cursor-loop",
				has_more: true,
			});

		render(FeedGrid, {
			props: {
				unreadOnly: true,
				excludeQueued: true,
				fetchFn,
				onSelectFeed: vi.fn(),
			},
		});

		await settle(400);
		expect(fetchFn).toHaveBeenCalledTimes(2);
	});
});
