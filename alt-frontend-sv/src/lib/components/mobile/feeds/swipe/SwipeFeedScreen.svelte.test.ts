import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ReaderOverlayRouteState } from "#lib/reader/types.js";
import type { RenderFeed } from "#lib/schema/feed.js";

let currentPageState = $state<{ reader?: ReaderOverlayRouteState }>({});
let currentPageURL = $state(
	new URL("http://localhost/feeds/swipe/visual-preview"),
);

const mockPushState = vi.fn(
	(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
		currentPageState = state;
		if (url) {
			currentPageURL = new URL(String(url), "http://localhost");
		}
	},
);
const mockReplaceState = vi.fn(
	(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
		currentPageState = state;
		if (url) {
			currentPageURL = new URL(String(url), "http://localhost");
		}
	},
);

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
	beforeNavigate: vi.fn(),
	pushState: (url: string | URL, state: { reader?: ReaderOverlayRouteState }) =>
		mockPushState(url, state),
	replaceState: (
		url: string | URL,
		state: { reader?: ReaderOverlayRouteState },
	) => mockReplaceState(url, state),
}));

vi.mock("$app/state", () => ({
	page: {
		get state() {
			return currentPageState;
		},
		get url() {
			return currentPageURL;
		},
		get params() {
			return {};
		},
	},
}));

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
}));

vi.mock("#lib/api/client/articles.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		getFeedContentOnTheFlyClient: getFeedContentOnTheFlyClientMock,
		getArticleSourceURLClient: vi
			.fn()
			.mockResolvedValue({ url: "", title: "" }),
	};
});

vi.mock("#lib/api/client/knowledge_home.js", () => ({
	trackHomeActionClient: vi.fn(() => Promise.resolve()),
}));

vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

vi.mock("#lib/utils/articlePrefetcher.js", () => ({
	articlePrefetcher: articlePrefetcherMock,
}));

vi.mock("#lib/utils/ogImageResolver.js", () => ({
	ogImageResolver: () => ({ resolve: () => Promise.resolve(null) }),
}));

import { trackHomeActionClient } from "#lib/api/client/knowledge_home.js";
import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import { recordReadingEvents } from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import ArticleReaderOverlay from "#lib/reader/ArticleReaderOverlay.svelte";
import { resetReaderOverlaySessions } from "#lib/reader/reader-overlay.svelte.js";
import { flushReadingEvents } from "#lib/reading/instrumentation.js";
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
	let backSpy: ReturnType<typeof vi.spyOn>;

	beforeEach(async () => {
		resetReaderOverlaySessions();
		currentPageState = {};
		currentPageURL = new URL("http://localhost/feeds/swipe/visual-preview");
		mockPushState.mockClear();
		mockReplaceState.mockClear();
		backSpy = vi.spyOn(history, "back").mockImplementation(() => {
			currentPageState = {};
			currentPageURL = new URL("http://localhost/feeds/swipe/visual-preview");
		});

		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 0,
			duplicates: 0,
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockReset();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
		});

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

		vi.mocked(trackHomeActionClient).mockClear();
		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);

		for (const fn of Object.values(articlePrefetcherMock)) {
			fn.mockClear();
		}
		articlePrefetcherMock.getCachedOgImage.mockReturnValue(null);
		articlePrefetcherMock.getCachedContent.mockReturnValue(null);
		articlePrefetcherMock.getCachedArticleId.mockReturnValue(null);
		articlePrefetcherMock.ensureContent.mockResolvedValue("<p>Body</p>");

		return () => {
			backSpy.mockRestore();
		};
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

	it("triggers visual-preview prefetchAhead=4 and batch image prefetch on articleId cache", async () => {
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

	it("opens the reader overlay on tap/Article without marking read and restores the same top card on close", async () => {
		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({ id: "feed-1", articleId: "art-1", title: "Top Card" }),
					makeFeed({
						id: "feed-2",
						articleId: "art-2",
						title: "Second Card",
						link: "https://example.com/dispatch-2",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		render(ArticleReaderOverlay);
		await settle(200);

		await page.getByRole("button", { name: /^article$/i }).click();

		await expect.element(page.getByTestId("reader-overlay")).toBeVisible();
		expect(currentPageURL.pathname).toBe("/articles/art-1");
		expect(updateFeedReadStatusClientMock).not.toHaveBeenCalled();
		expect(trackHomeActionClient).not.toHaveBeenCalled();

		await page.getByTestId("reader-overlay-close").click();

		await expect
			.element(page.getByTestId("reader-overlay"))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByRole("heading", { name: "Top Card" }))
			.toBeVisible();
	});
});
