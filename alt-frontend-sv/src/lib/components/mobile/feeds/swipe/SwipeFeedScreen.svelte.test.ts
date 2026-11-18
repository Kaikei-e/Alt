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
import {
	getReadTodayQueue,
	QueueArticleOutcome,
	queueArticle,
	recordReadingEvents,
	unqueueArticle,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import ArticleReaderOverlay from "#lib/reader/ArticleReaderOverlay.svelte";
import { resetReaderOverlaySessions } from "#lib/reader/reader-overlay.svelte.js";
import { flushReadingEvents } from "#lib/reading/instrumentation.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
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
		readTodayQueue.reset();
		currentPageState = {};
		currentPageURL = new URL("http://localhost/feeds/swipe/visual-preview");
		mockPushState.mockClear();
		mockReplaceState.mockClear();
		backSpy = vi.spyOn(history, "back").mockImplementation(() => {
			currentPageState = {};
			currentPageURL = new URL("http://localhost/feeds/swipe/visual-preview");
		});

		vi.mocked(getReadTodayQueue).mockReset();
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [],
			cap: 7,
			readCountSince: 0,
		});
		vi.mocked(queueArticle).mockReset();
		vi.mocked(queueArticle).mockResolvedValue({
			outcome: QueueArticleOutcome.QUEUED,
			items: [],
			cap: 7,
		});
		vi.mocked(unqueueArticle).mockReset();
		vi.mocked(unqueueArticle).mockResolvedValue(undefined);

		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 0,
			duplicates: 0,
			readArticleIds: [],
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockReset();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
			readArticleIds: [],
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

	it("opens the reader overlay on card tap without marking read and restores the same top card on close", async () => {
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

		await page.getByText("Consensus and replication notes").click();

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

	it("renders Pass and Read today buttons under the card that mirror left/right swipes and support UNDO", async () => {
		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({ id: "feed-1", articleId: "art-1", title: "Card One" }),
					makeFeed({
						id: "feed-2",
						articleId: "art-2",
						title: "Card Two",
						link: "https://example.com/dispatch-2",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
					makeFeed({
						id: "feed-3",
						articleId: "art-3",
						title: "Card Three",
						link: "https://example.com/dispatch-3",
						normalizedUrl: "https://example.com/dispatch-3",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		const passBtn = page.getByRole("button", { name: "Pass" });
		const queueBtn = page.getByRole("button", { name: "Read today" });
		await expect.element(passBtn).toBeVisible();
		await expect.element(queueBtn).toBeVisible();
		await expect.element(queueBtn).not.toHaveAttribute("aria-disabled", "true");

		// Tap Read today on Card One (held in pending slot, not sent yet)
		await queueBtn.click();
		await expect
			.element(page.getByRole("heading", { name: "Card Two" }))
			.toBeVisible();
		await expect
			.element(page.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 1");
		expect(queueArticle).not.toHaveBeenCalled();
		expect(updateFeedReadStatusClientMock).not.toHaveBeenCalled();

		// Undo returns Card One
		await page.getByTestId("dispatch-undo").click();
		await expect
			.element(page.getByRole("heading", { name: "Card One" }))
			.toBeVisible();
		await expect
			.element(page.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 0");

		// Tap Read today on Card One, then Pass on Card Two -> flushes Card One as queue (no mark read)
		await queueBtn.click();
		await passBtn.click();
		await expect
			.element(page.getByRole("heading", { name: "Card Three" }))
			.toBeVisible();
		await expect
			.element(page.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 2");

		await vi.waitFor(() => expect(queueArticle).toHaveBeenCalledTimes(1));
		expect(queueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: "art-1",
			surface: 2,
		});
		expect(updateFeedReadStatusClientMock).not.toHaveBeenCalled();

		// Pass Card Three (last card) -> flushes Card Two as pass and Card Three as pass
		await passBtn.click();
		await vi.waitFor(() =>
			expect(updateFeedReadStatusClientMock).toHaveBeenCalledTimes(2),
		);
	});

	it("refuses Read today immediately when remainingSlots is 0 and displays cap notice", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: Array.from({ length: 7 }, (_, i) => ({
				articleId: `queued-${i}`,
				title: `Queued ${i}`,
				url: `https://example.com/queued-${i}`,
				sourceName: "Source",
				imageUrl: "",
				queuedAt: new Date(),
				expiresAt: new Date(Date.now() + 3600_000),
			})),
			cap: 7,
			readCountSince: 0,
		});

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({
						id: "feed-1",
						articleId: "art-1",
						title: "Full Deck Card",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		const queueBtn = page.getByTestId("swipe-queue-button");
		await expect.element(queueBtn).toHaveAttribute("aria-disabled", "true");
		expect(queueBtn.element().hasAttribute("disabled")).toBe(false);
		(queueBtn.element() as HTMLButtonElement).click();

		await expect
			.element(page.getByTestId("swipe-notice"))
			.toHaveTextContent("Read today is full (7). Read or remove one first.");
		await expect
			.element(page.getByRole("heading", { name: "Full Deck Card" }))
			.toBeVisible();
		expect(queueArticle).not.toHaveBeenCalled();
	});

	it("resolves articleId at flush time when missing and returns card to deck with notice on error or QUEUE_FULL", async () => {
		getFeedContentOnTheFlyClientMock.mockResolvedValueOnce({
			content: "<p>Resolved body</p>",
			article_id: "resolved-from-fetch",
			og_image_url: "",
			og_image_proxy_url: "",
		});
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUE_FULL,
			items: [],
			cap: 7,
		});

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({
						id: "feed-no-id",
						articleId: undefined,
						title: "Needs Resolution",
						link: "https://example.com/needs-id",
						normalizedUrl: "https://example.com/needs-id",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		// Last card -> flushes immediately
		await page.getByRole("button", { name: "Read today" }).click();

		await vi.waitFor(() =>
			expect(getFeedContentOnTheFlyClientMock).toHaveBeenCalledWith(
				"https://example.com/needs-id",
			),
		);
		await vi.waitFor(() =>
			expect(queueArticle).toHaveBeenCalledWith(expect.anything(), {
				articleId: "resolved-from-fetch",
				surface: 2,
			}),
		);

		await expect
			.element(page.getByTestId("swipe-notice"))
			.toHaveTextContent("Read today is full (7). Read or remove one first.");
		await expect
			.element(page.getByRole("heading", { name: "Needs Resolution" }))
			.toBeVisible();
	});

	it("filters queued articles from the Swipe deck by articleId or normalized URL without marking read", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [
				{
					articleId: "art-queued",
					title: "Queued by ID",
					url: "https://other.example/queued",
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
				{
					articleId: "art-different",
					title: "Queued by URL",
					url: "https://example.com/dispatch-2/",
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
			readCountSince: 0,
		});

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({
						id: "feed-1",
						articleId: "art-queued",
						title: "Hidden By ID",
						link: "https://example.com/dispatch-1",
						normalizedUrl: "https://example.com/dispatch-1",
					}),
					makeFeed({
						id: "feed-2",
						articleId: "art-2",
						title: "Hidden By URL",
						link: "https://example.com/dispatch-2?utm_source=rss",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
					makeFeed({
						id: "feed-3",
						articleId: "art-3",
						title: "Visible Card",
						link: "https://example.com/dispatch-3",
						normalizedUrl: "https://example.com/dispatch-3",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});

		await expect
			.element(page.getByRole("heading", { name: "Visible Card" }))
			.toBeVisible();
		expect(updateFeedReadStatusClientMock).not.toHaveBeenCalled();
	});

	it("ignores Pass/Read today button presses while a swipe exit animation is in flight", async () => {
		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({ id: "feed-1", articleId: "art-1", title: "Card One" }),
					makeFeed({
						id: "feed-2",
						articleId: "art-2",
						title: "Card Two",
						link: "https://example.com/dispatch-2",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
					makeFeed({
						id: "feed-3",
						articleId: "art-3",
						title: "Card Three",
						link: "https://example.com/dispatch-3",
						normalizedUrl: "https://example.com/dispatch-3",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		const card = page.getByTestId("visual-preview-card").element();
		card.dispatchEvent(
			new CustomEvent("swipe", { detail: { direction: "left" } }),
		);

		// Immediately click Read today while Card One's swipe exit animation is still running
		const queueBtn = page
			.getByTestId("swipe-queue-button")
			.element() as HTMLButtonElement;
		queueBtn.click();

		await expect
			.element(page.getByRole("heading", { name: "Card Two" }))
			.toBeVisible();
		await settle(1200);
		await expect
			.element(page.getByRole("heading", { name: "Card Two" }))
			.toBeVisible();
		await expect
			.element(page.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 1");
		expect(queueArticle).not.toHaveBeenCalled();

		// Passing Card Two flushes Card One: proves the swipe-left (pass) won, not the ignored Read today button
		await page.getByTestId("swipe-pass-button").click();
		await vi.waitFor(() =>
			expect(updateFeedReadStatusClientMock).toHaveBeenCalledWith(
				"https://example.com/dispatch-1",
			),
		);
		expect(queueArticle).not.toHaveBeenCalled();
	});

	it("ignores a delayed swipe onDismiss when the swiped card is no longer active", async () => {
		const artOneId = "01938e82-7c00-7a7b-9b10-000000000401";
		const artTwoId = "01938e82-7c00-7a7b-9b10-000000000402";
		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({ id: "feed-1", articleId: artOneId, title: "Card One" }),
					makeFeed({
						id: "feed-2",
						articleId: artTwoId,
						title: "Card Two",
						link: "https://example.com/dispatch-2",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		const card = page.getByTestId("visual-preview-card").element();
		card.dispatchEvent(
			new CustomEvent("swipe", { detail: { direction: "left" } }),
		);

		// Card One disappears from the remaining deck mid-animation (e.g. queued elsewhere), making Card Two active
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUED,
			items: [
				{
					articleId: artOneId,
					title: "Card One",
					url: "https://example.com/dispatch-1",
					sourceName: "Alice",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
		});
		await readTodayQueue.queue({
			articleId: artOneId,
			surface: 2,
		});

		await settle(500);
		// Card Two must still be active and dispatch counter must remain 0
		await expect
			.element(page.getByRole("heading", { name: "Card Two" }))
			.toBeVisible();
		await expect
			.element(page.getByTestId("dispatch-counter"))
			.toHaveTextContent("№ 0");
	});

	it("resolves missing articleId immediately when a right swipe is accepted before commit time", async () => {
		getFeedContentOnTheFlyClientMock.mockResolvedValueOnce({
			content: "<p>Resolved early</p>",
			article_id: "01938e82-7c00-7a7b-9b10-000000000701",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({
						id: "feed-no-id",
						articleId: undefined,
						title: "Resolve Early",
						link: "https://example.com/resolve-early",
						normalizedUrl: "https://example.com/resolve-early",
					}),
					makeFeed({
						id: "feed-2",
						articleId: "01938e82-7c00-7a7b-9b10-000000000702",
						title: "Second Card",
						link: "https://example.com/second",
						normalizedUrl: "https://example.com/second",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		await page.getByTestId("swipe-queue-button").click();

		// Article ID resolution starts immediately upon acceptance, before the next card is dismissed
		await vi.waitFor(() =>
			expect(getFeedContentOnTheFlyClientMock).toHaveBeenCalledTimes(1),
		);
		expect(queueArticle).not.toHaveBeenCalled();

		await page.getByTestId("swipe-pass-button").click();
		await vi.waitFor(() => expect(queueArticle).toHaveBeenCalledTimes(1));
		expect(queueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: "01938e82-7c00-7a7b-9b10-000000000701",
			surface: 2,
		});
		expect(getFeedContentOnTheFlyClientMock).toHaveBeenCalledTimes(1);
	});

	it("commits pending pass and queue on visibilitychange hidden with keepalive", async () => {
		const visibilitySpy = vi
			.spyOn(document, "visibilityState", "get")
			.mockReturnValue("hidden");
		try {
			render(SwipeFeedScreen, {
				props: {
					initialFeeds: [
						makeFeed({
							id: "feed-1",
							articleId: "01938e82-7c00-7a7b-9b10-000000000711",
							title: "Card One",
						}),
						makeFeed({
							id: "feed-2",
							articleId: "01938e82-7c00-7a7b-9b10-000000000712",
							title: "Card Two",
							link: "https://example.com/dispatch-2",
							normalizedUrl: "https://example.com/dispatch-2",
						}),
						makeFeed({
							id: "feed-3",
							articleId: "01938e82-7c00-7a7b-9b10-000000000713",
							title: "Card Three",
							link: "https://example.com/dispatch-3",
							normalizedUrl: "https://example.com/dispatch-3",
						}),
					],
					initialNextCursor: null,
					mode: "visual-preview",
				},
			});
			await settle(200);

			// Pending pass flushed on visibilitychange -> hidden with keepalive
			await page.getByTestId("swipe-pass-button").click();
			expect(updateFeedReadStatusClientMock).not.toHaveBeenCalled();

			document.dispatchEvent(new Event("visibilitychange"));
			await vi.waitFor(() =>
				expect(updateFeedReadStatusClientMock).toHaveBeenCalledWith(
					"https://example.com/dispatch-1",
					{ keepalive: true },
				),
			);

			// Pending queue flushed on visibilitychange -> hidden
			await page.getByTestId("swipe-queue-button").click();
			expect(queueArticle).not.toHaveBeenCalled();

			document.dispatchEvent(new Event("visibilitychange"));
			await vi.waitFor(() => expect(queueArticle).toHaveBeenCalledTimes(1));
		} finally {
			visibilitySpy.mockRestore();
		}
	});

	it("treats UNSPECIFIED QueueArticleOutcome as failure and sets pendingNotice when unmounted", async () => {
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.UNSPECIFIED,
			items: [],
			cap: 7,
		});

		const { unmount } = render(SwipeFeedScreen, {
			props: {
				initialFeeds: [
					makeFeed({
						id: "feed-1",
						articleId: "01938e82-7c00-7a7b-9b10-000000000721",
						title: "Unspecified Card",
					}),
					makeFeed({
						id: "feed-2",
						articleId: "01938e82-7c00-7a7b-9b10-000000000722",
						title: "Next Card",
						link: "https://example.com/dispatch-2",
						normalizedUrl: "https://example.com/dispatch-2",
					}),
					makeFeed({
						id: "feed-3",
						articleId: "01938e82-7c00-7a7b-9b10-000000000723",
						title: "Third Card",
						link: "https://example.com/dispatch-3",
						normalizedUrl: "https://example.com/dispatch-3",
					}),
				],
				initialNextCursor: null,
				mode: "visual-preview",
			},
		});
		await settle(200);

		// First test UNSPECIFIED while mounted -> restores card and shows notice
		await page.getByTestId("swipe-queue-button").click();
		await page.getByTestId("swipe-pass-button").click();

		await vi.waitFor(() =>
			expect(page.getByTestId("swipe-notice")).toBeVisible(),
		);
		await expect
			.element(page.getByRole("heading", { name: "Unspecified Card" }))
			.toBeVisible();

		// Now queue and unmount immediately before commit rejects -> sets readTodayQueue.pendingNotice
		vi.mocked(queueArticle).mockRejectedValueOnce(new Error("network down"));
		await page.getByTestId("swipe-queue-button").click();
		unmount();

		await vi.waitFor(() =>
			expect(readTodayQueue.pendingNotice).toContain("Unspecified Card"),
		);
	});
});
