import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TagTrailArticle, TagTrailTag } from "#lib/connect/index.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import { assertDefined } from "#lib/test-utils/assert.js";

const {
	fetchArticlesByTagMock,
	fetchRandomFeedMock,
	streamArticleTagsMock,
	createClientTransportMock,
} = vi.hoisted(() => ({
	fetchArticlesByTagMock: vi.fn(),
	fetchRandomFeedMock: vi.fn(),
	streamArticleTagsMock: vi.fn(() => new AbortController()),
	createClientTransportMock: vi.fn(() => ({})),
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: createClientTransportMock,
	fetchArticlesByTag: fetchArticlesByTagMock,
	fetchRandomFeed: fetchRandomFeedMock,
	streamArticleTags: streamArticleTagsMock,
}));

vi.mock("#lib/reader/reader-overlay.svelte.js", () => ({
	openReaderOverlay: vi.fn(),
	replaceReaderOverlayItems: vi.fn(),
	closeReaderOverlay: vi.fn(),
	updateReaderOverlayArticleId: vi.fn(),
	navigateReaderOverlay: vi.fn(),
	getReaderOverlaySession: vi.fn(),
	resetReaderOverlaySessions: vi.fn(),
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const TAGS: TagTrailTag[] = [
	{ id: "tag-1", name: "AI" },
	{ id: "tag-2", name: "Systems" },
];

const INITIAL_FEED = {
	id: "feed-1",
	url: "https://example.com/feed",
	title: "Random Trail Feed",
	description: "Curated systems and AI articles",
	tags: TAGS,
};

const ARTICLES: TagTrailArticle[] = [
	{
		id: "trail-art-1",
		title: "AI Trends in 2026",
		link: "https://example.com/ai-trends",
		publishedAt: "2026-04-01T12:00:00Z",
		feedTitle: "TechBlog",
	},
];

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/feeds/tag-trail page", () => {
	beforeEach(() => {
		vi.mocked(openReaderOverlay).mockReset();
		fetchArticlesByTagMock.mockReset();
		fetchArticlesByTagMock.mockResolvedValue({
			articles: ARTICLES,
			nextCursor: null,
			hasMore: false,
		});
		fetchRandomFeedMock.mockReset();
		fetchRandomFeedMock.mockResolvedValue(INITIAL_FEED);
		streamArticleTagsMock.mockReset();
		streamArticleTagsMock.mockImplementation(() => new AbortController());
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("rejects unsafe javascript: and data: URLs in feed and article links on desktop", async () => {
		fetchArticlesByTagMock.mockResolvedValueOnce({
			articles: [
				{
					id: "unsafe-art",
					title: "Unsafe Article Link",
					link: "javascript:alert(1)",
					publishedAt: "2026-04-01T12:00:00Z",
					feedTitle: "Untrusted Feed",
				},
				{
					id: "safe-art",
					title: "Safe Article Link",
					link: "https://example.com/safe-article",
					publishedAt: "2026-04-01T12:00:00Z",
					feedTitle: "Trusted Feed",
				},
			],
			nextCursor: null,
			hasMore: false,
		});

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		const { container } = render(Page, {
			props: {
				data: {
					initialFeed: {
						...INITIAL_FEED,
						url: "javascript:alert(1)",
					},
				},
			},
		});
		await settle(250);

		expect(container.querySelector('a[href^="javascript:"]')).toBeNull();

		await page.getByRole("button", { name: "AI" }).click();
		await settle(250);

		expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
		expect(
			container.querySelector('a[href="https://example.com/safe-article"]'),
		).not.toBeNull();
	});

	it("preserves trail hops and loaded articles across 393x851 <-> 851x393 viewport rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page, { props: { data: { initialFeed: INITIAL_FEED } } });
		await settle(300);

		await page.getByRole("button", { name: "AI" }).click();
		await settle(300);

		await expect
			.element(page.getByText("AI Trends in 2026"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("tag-trail-breadcrumb"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(300);

		await expect
			.element(page.getByText("AI Trends in 2026"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("trail-breadcrumb"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(300);

		await expect
			.element(page.getByText("AI Trends in 2026"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("tag-trail-breadcrumb"))
			.toBeInTheDocument();
		expect(fetchArticlesByTagMock).toHaveBeenCalledTimes(1);
	});

	it("displays refreshError on mobile when fetching a random feed fails", async () => {
		fetchRandomFeedMock.mockRejectedValueOnce(new Error("RPC error"));

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page, { props: { data: { initialFeed: INITIAL_FEED } } });
		await settle(250);

		await page
			.getByRole("button", { name: /get another random feed/i })
			.click();
		await settle(250);

		await expect
			.element(page.getByText("Failed to load feed. Please try again."))
			.toBeInTheDocument();
	});

	it("displays 45-second tag generation timeout error on mobile", async () => {
		vi.useFakeTimers();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page, {
			props: {
				data: {
					initialFeed: {
						id: "feed-stream",
						url: "https://example.com/stream",
						title: "Streaming Feed",
						tags: [],
						latestArticleId: "art-stream-1",
					},
				},
			},
		});

		await vi.advanceTimersByTimeAsync(45000);

		await expect
			.element(page.getByText("Tag generation timed out. Please refresh."))
			.toBeInTheDocument();
	});

	it("opens the unified reader overlay from desktop and mobile tag-trail article cards", async () => {
		await page.viewport(1280, 800);
		render(Page, { props: { data: { initialFeed: INITIAL_FEED } } });
		await settle(250);

		await page.getByRole("button", { name: "AI" }).click();
		await settle(250);

		const firstArticle = assertDefined(ARTICLES[0]);

		await page
			.getByRole("button", { name: `Show details for ${firstArticle.title}` })
			.click();

		expect(openReaderOverlay).toHaveBeenCalledTimes(1);
		const desktopCtx = vi.mocked(openReaderOverlay).mock.calls[0]?.[0];
		expect(desktopCtx?.surface).toBe(ReadingSurface.OTHER);
		expect(desktopCtx?.currentIndex).toBe(0);
		expect(desktopCtx?.items?.[0]?.url).toBe(firstArticle.link);

		vi.mocked(openReaderOverlay).mockReset();
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(250);

		await page
			.getByRole("button", { name: `Show details for ${firstArticle.title}` })
			.click();

		expect(openReaderOverlay).toHaveBeenCalledTimes(1);
		const mobileCtx = vi.mocked(openReaderOverlay).mock.calls[0]?.[0];
		expect(mobileCtx?.surface).toBe(ReadingSurface.OTHER);
		expect(mobileCtx?.currentIndex).toBe(0);
		expect(mobileCtx?.items?.[0]?.url).toBe(firstArticle.link);
	});
});
