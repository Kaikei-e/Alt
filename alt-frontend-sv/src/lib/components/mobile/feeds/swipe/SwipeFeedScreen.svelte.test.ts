import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { RenderFeed } from "#lib/schema/feed.js";

const {
	batchPrefetchImagesClientMock,
	getFeedContentOnTheFlyClientMock,
	getFeedsWithCursorClientMock,
	getReadFeedsWithCursorClientMock,
	listSubscriptionsClientMock,
	updateFeedReadStatusClientMock,
	articlePrefetcherMock,
} = vi.hoisted(() => ({
	batchPrefetchImagesClientMock: vi.fn(),
	getFeedContentOnTheFlyClientMock: vi.fn(),
	getFeedsWithCursorClientMock: vi.fn(),
	getReadFeedsWithCursorClientMock: vi.fn(),
	listSubscriptionsClientMock: vi.fn(),
	updateFeedReadStatusClientMock: vi.fn(),
	articlePrefetcherMock: {
		triggerPrefetch: vi.fn(),
		setOnOgImageFetched: vi.fn(),
		setOnArticleIdCached: vi.fn(),
		getCachedOgImage: vi.fn<() => string | null>(() => null),
		getCachedContent: vi.fn<() => string | null>(() => null),
		getCachedArticleId: vi.fn<() => string | null>(() => null),
		ensureContent: vi.fn(() => Promise.resolve("<p>Body</p>")),
		seedCache: vi.fn(),
		markAsDismissed: vi.fn(),
	},
}));

vi.mock("#lib/api/client/index.js", () => ({
	batchPrefetchImagesClient: batchPrefetchImagesClientMock,
	getFeedContentOnTheFlyClient: getFeedContentOnTheFlyClientMock,
	getFeedsWithCursorClient: getFeedsWithCursorClientMock,
	getReadFeedsWithCursorClient: getReadFeedsWithCursorClientMock,
	listSubscriptionsClient: listSubscriptionsClientMock,
	updateFeedReadStatusClient: updateFeedReadStatusClientMock,
	registerFavoriteFeedClient: vi.fn(),
	summarizeArticleClient: vi.fn(),
}));

vi.mock("#lib/api/client/knowledge_home.js", () => ({
	trackHomeActionClient: vi.fn(() => Promise.resolve()),
}));

vi.mock("#lib/utils/articlePrefetcher.js", () => ({
	articlePrefetcher: articlePrefetcherMock,
}));

vi.mock("#lib/utils/ogImageResolver.js", () => ({
	ogImageResolver: () => ({ resolve: () => Promise.resolve(null) }),
}));

import SwipeFeedScreen from "./SwipeFeedScreen.svelte";

function makeFeed(overrides: Partial<RenderFeed> = {}): RenderFeed {
	return {
		id: "feed-1",
		feedId: "source-1",
		articleId: "art-1",
		title: "Distributed Systems Dispatch",
		description: "Consensus and replication notes",
		link: "https://example.com/dispatch-1",
		normalizedUrl: "https://example.com/dispatch-1",
		published: "2026-04-01T12:00:00Z",
		created_at: "2026-04-01T12:00:00Z",
		author: "Alice",
		publishedAtFormatted: "Apr 1, 2026",
		mergedTagsLabel: "distributed-systems",
		excerpt: "Consensus and replication notes",
		isRead: false,
		...overrides,
	} as RenderFeed;
}

async function settle(ms = 200) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("SwipeFeedScreen", () => {
	beforeEach(() => {
		batchPrefetchImagesClientMock.mockReset();
		batchPrefetchImagesClientMock.mockResolvedValue([]);

		getFeedContentOnTheFlyClientMock.mockReset();
		getFeedContentOnTheFlyClientMock.mockResolvedValue({
			content: "<p>Full article content</p>",
			article_id: "art-1",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		getFeedsWithCursorClientMock.mockReset();
		getFeedsWithCursorClientMock.mockResolvedValue({
			data: [makeFeed()],
			next_cursor: null,
			has_more: false,
		});

		getReadFeedsWithCursorClientMock.mockReset();
		getReadFeedsWithCursorClientMock.mockResolvedValue({
			data: [],
			next_cursor: null,
			has_more: false,
		});

		listSubscriptionsClientMock.mockReset();
		listSubscriptionsClientMock.mockResolvedValue([]);

		updateFeedReadStatusClientMock.mockReset();
		updateFeedReadStatusClientMock.mockResolvedValue(undefined);

		for (const fn of Object.values(articlePrefetcherMock)) {
			fn.mockClear();
		}
		articlePrefetcherMock.getCachedOgImage.mockReturnValue(null);
		articlePrefetcherMock.getCachedContent.mockReturnValue(null);
		articlePrefetcherMock.getCachedArticleId.mockReturnValue(null);
		articlePrefetcherMock.ensureContent.mockResolvedValue("<p>Body</p>");
	});

	it("clears initial loading state and renders Retry UI when initial feed load fails", async () => {
		getFeedsWithCursorClientMock.mockRejectedValueOnce(
			new Error("Upstream unavailable"),
		);

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [],
			},
		});
		await settle(250);

		await expect
			.element(page.getByText("Error loading feeds"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Failed to load feeds"))
			.toBeInTheDocument();

		const retryBtn = page.getByRole("button", { name: "Retry" });
		await expect.element(retryBtn).toBeInTheDocument();

		await retryBtn.click();
		await settle(250);

		await expect
			.element(page.getByText("Distributed Systems Dispatch"))
			.toBeInTheDocument();
	});

	it("guards handleArticleIdResolved against redundant feeds reassignment when articleId is unchanged", async () => {
		articlePrefetcherMock.getCachedContent.mockReturnValue(null);
		articlePrefetcherMock.getCachedArticleId.mockReturnValue("art-1");

		const { unmount } = render(SwipeFeedScreen, {
			props: {
				initialFeeds: [makeFeed({ articleId: "art-1" })],
				initialNextCursor: null,
			},
		});
		await settle(200);

		// Because feed.articleId is already "art-1", handleArticleIdResolved must not
		// rebuild `feeds` or re-trigger the prefetch effect after ensureContent resolves.
		expect(articlePrefetcherMock.triggerPrefetch).toHaveBeenCalledTimes(1);
		unmount();

		articlePrefetcherMock.triggerPrefetch.mockClear();
		articlePrefetcherMock.getCachedArticleId.mockReturnValue("art-new-99");

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [makeFeed({ articleId: undefined })],
				initialNextCursor: null,
			},
		});
		await settle(200);

		// When articleId changes from undefined -> "art-new-99" after ensureContent resolves,
		// `feeds` is updated and the prefetch effect runs a second time with the resolved articleId.
		expect(articlePrefetcherMock.triggerPrefetch).toHaveBeenCalledTimes(2);
		const secondCallFeeds = articlePrefetcherMock.triggerPrefetch.mock
			.calls[1]?.[0] as RenderFeed[];
		expect(secondCallFeeds[0]?.articleId).toBe("art-new-99");
	});

	it("distinguishes default mode (prefetchAhead=2) from visual-preview mode (prefetchAhead=4 and batch image prefetch)", async () => {
		const { unmount } = render(SwipeFeedScreen, {
			props: {
				initialFeeds: [makeFeed()],
				initialNextCursor: null,
				mode: "default",
			},
		});
		await settle(200);

		expect(articlePrefetcherMock.triggerPrefetch).toHaveBeenCalledWith(
			expect.any(Array),
			0,
			2,
		);
		expect(articlePrefetcherMock.setOnArticleIdCached).not.toHaveBeenCalled();
		unmount();

		articlePrefetcherMock.triggerPrefetch.mockClear();
		articlePrefetcherMock.setOnArticleIdCached.mockClear();

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [makeFeed()],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		expect(articlePrefetcherMock.triggerPrefetch).toHaveBeenCalledWith(
			expect.any(Array),
			0,
			4,
		);
		expect(articlePrefetcherMock.setOnArticleIdCached).toHaveBeenCalledWith(
			expect.any(Function),
		);

		const onArticleIdCached = articlePrefetcherMock.setOnArticleIdCached.mock
			.calls[0]?.[0] as
			| ((feedUrl: string, articleId: string) => void)
			| undefined;
		onArticleIdCached?.("https://example.com/dispatch-1", "art-vp-1");
		await settle(100);

		expect(batchPrefetchImagesClientMock).toHaveBeenCalledWith(["art-vp-1"]);
	});
});
