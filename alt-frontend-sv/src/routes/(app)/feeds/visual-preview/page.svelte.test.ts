/**
 * `/feeds/visual-preview` across a rotation and reader overlay integration.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ReaderOverlayRouteState } from "#lib/reader/types.js";

let currentPageState = $state<{ reader?: ReaderOverlayRouteState }>({});
let currentPageURL = $state(new URL("http://localhost/feeds/visual-preview"));

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

vi.mock("#lib/api/client/feeds.js", { spy: true });
vi.mock("#lib/api/client/articles.js", { spy: true });
vi.mock("#lib/api/client/knowledge_home.js", { spy: true });
vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

// The tiles resolve OG images through a shared resolver that would otherwise
// fire a Connect-RPC at whatever is listening on the dev server.
vi.mock("#lib/utils/ogImageResolver.js", () => ({
	ogImageResolver: () => ({ resolve: () => Promise.resolve(null) }),
}));

import {
	batchPrefetchImagesClient,
	getFeedContentOnTheFlyClient,
} from "#lib/api/client/articles.js";
import {
	getAllFeedsWithCursorClient,
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "#lib/api/client/feeds.js";
import { trackHomeActionClient } from "#lib/api/client/knowledge_home.js";
import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import {
	getReadTodayQueue,
	recordReadingEvents,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import { MARK_AS_READ_FAILED_MESSAGE } from "#lib/feeds/mark-as-read-feedback.js";
import ArticleReaderOverlay from "#lib/reader/ArticleReaderOverlay.svelte";
import { resetReaderOverlaySessions } from "#lib/reader/reader-overlay.svelte.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import type { RenderFeed } from "#lib/schema/feed.js";
import { createRenderFeed } from "../../../../../tests/fixtures/feeds";
import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const FEEDS: RenderFeed[] = Array.from({ length: 6 }, (_, i) =>
	createRenderFeed(`feed-${i + 1}`, `https://example.com/feed-${i + 1}`),
);

/** The tile the mark-as-read cases act on. */
const FIRST = FEEDS[0] as RenderFeed;

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

function gridRoot(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".wire-container");
}

function cardLabels(): string[] {
	const grid = gridRoot()?.querySelector('[class*="grid-cols"]');
	if (!grid) return [];
	return Array.from(grid.children).map(
		(child) =>
			child.querySelector("[aria-label]")?.getAttribute("aria-label") ?? "",
	);
}

const openLabel = (feed: RenderFeed) => `Open ${feed.title}`;

function feedFetches(): number {
	return vi.mocked(getAllFeedsWithCursorClient).mock.calls.length;
}

describe("Visual Preview page continuity across a rotation", () => {
	let backSpy: ReturnType<typeof vi.spyOn>;

	beforeEach(async () => {
		resetReaderOverlaySessions();
		readTodayQueue.reset();
		vi.mocked(getReadTodayQueue).mockReset();
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [],
			cap: 7,
			readCountSince: 0,
		});
		currentPageState = {};
		currentPageURL = new URL("http://localhost/feeds/visual-preview");
		mockPushState.mockClear();
		mockReplaceState.mockClear();

		backSpy = vi.spyOn(history, "back").mockImplementation(() => {
			currentPageState = {};
			currentPageURL = new URL("http://localhost/feeds/visual-preview");
		});

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
		vi.mocked(getAllFeedsWithCursorClient).mockReset();
		vi.mocked(getAllFeedsWithCursorClient).mockResolvedValue({
			data: FEEDS,
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<ReturnType<typeof getAllFeedsWithCursorClient>>);

		vi.mocked(listSubscriptionsClient).mockReset();
		vi.mocked(listSubscriptionsClient).mockResolvedValue([]);

		vi.mocked(batchPrefetchImagesClient).mockReset();
		vi.mocked(batchPrefetchImagesClient).mockResolvedValue([]);

		vi.mocked(getFeedContentOnTheFlyClient).mockReset();
		vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
			content: "<p>Gallery article body</p>",
			article_id: "art-1",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		vi.mocked(updateFeedReadStatusClient).mockReset();
		vi.mocked(updateFeedReadStatusClient).mockResolvedValue(
			undefined as unknown as Awaited<
				ReturnType<typeof updateFeedReadStatusClient>
			>,
		);

		vi.mocked(trackHomeActionClient).mockReset();
		vi.mocked(trackHomeActionClient).mockResolvedValue(undefined);
		vi.mocked(getItemBranches).mockReset();
		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);

		return () => {
			backSpy.mockRestore();
		};
	});

	it("opens the reader overlay via shallow routing without marking read on open", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		render(ArticleReaderOverlay);
		await settle(400);

		await page.getByRole("button", { name: openLabel(FIRST) }).click();

		await expect.element(page.getByTestId("reader-overlay")).toBeVisible();
		expect(mockPushState).toHaveBeenCalledTimes(1);
		await vi.waitFor(() =>
			expect(currentPageURL.pathname).toBe("/articles/art-1"),
		);
		expect(trackHomeActionClient).not.toHaveBeenCalled();
		expect(updateFeedReadStatusClient).not.toHaveBeenCalled();
	});

	it("keeps an article the reader marked read out of the grid after a rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		render(ArticleReaderOverlay);
		await settle(400);

		expect(cardLabels()).toHaveLength(FEEDS.length);

		// Open the first tile and mark it read inside the overlay.
		await page.getByRole("button", { name: openLabel(FIRST) }).click();
		await page.getByRole("button", { name: "Mark as Read" }).click();
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		const afterRead = cardLabels().length;

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(afterRead);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(afterRead);
	});

	it("keeps the same grid element across a 851 -> 393 -> 851 round trip", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle(400);

		const before = gridRoot();
		expect(before).not.toBeNull();
		(before as HTMLElement).dataset.identityProbe = "landscape";

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		const after = gridRoot();
		expect(after).toBe(before);
		expect(after?.dataset.identityProbe).toBe("landscape");
	});

	it("does not re-fetch the wire, or lose cards, when the phone is rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		const fetchesBefore = feedFetches();
		const cardsBefore = cardLabels().length;
		expect(cardsBefore).toBe(FEEDS.length);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		expect(cardLabels()).toHaveLength(cardsBefore);
		expect(feedFetches()).toBe(fetchesBefore);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);

		expect(cardLabels()).toHaveLength(cardsBefore);
		expect(feedFetches()).toBe(fetchesBefore);
	});

	it("surfaces an error toast, announces to aria-live, and keeps the card when overlay mark-as-read fails", async () => {
		vi.mocked(updateFeedReadStatusClient).mockRejectedValueOnce(
			new Error("Network error"),
		);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		render(ArticleReaderOverlay);
		await settle(400);

		expect(cardLabels()).toContain(openLabel(FIRST));

		await page.getByRole("button", { name: openLabel(FIRST) }).click();
		await page.getByRole("button", { name: "Mark as Read" }).click();
		await settle(250);

		const live = document.querySelector('.sr-only[aria-live="assertive"]');
		expect(live?.textContent?.trim()).toBe(MARK_AS_READ_FAILED_MESSAGE);
		const toastMessage = document.querySelector(
			".toast-kind-error .toast-message",
		);
		expect(toastMessage?.textContent?.trim()).toBe(MARK_AS_READ_FAILED_MESSAGE);
		expect(cardLabels()).toContain(openLabel(FIRST));
	});

	it("enqueues one gallery surface_viewed event on mount", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(100);

		expect(recordReadingEvents).not.toHaveBeenCalled();

		await flushReadingEvents();

		expect(recordReadingEvents).toHaveBeenCalledTimes(1);
		const events = vi.mocked(recordReadingEvents).mock.calls[0]?.[1] ?? [];
		const surfaceViews = events.filter(
			(e) => e.event?.case === "surfaceViewed",
		);
		expect(surfaceViews).toHaveLength(1);
		expect(surfaceViews[0]?.event?.value).toEqual(
			expect.objectContaining({ surface: ReadingSurface.GALLERY }),
		);
	});

	it("hides queued articles from the Gallery without marking them read and restores them when unqueued", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValueOnce({
			items: [
				{
					articleId: FIRST.articleId ?? "art-1",
					title: FIRST.title,
					url: FIRST.normalizedUrl,
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
			readCountSince: 0,
		});

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(FEEDS.length - 1);
		expect(updateFeedReadStatusClient).not.toHaveBeenCalled();

		// When the item is removed/expired and the queue refreshes, it shows up again
		vi.mocked(getReadTodayQueue).mockResolvedValueOnce({
			items: [],
			cap: 7,
			readCountSince: 0,
		});
		await readTodayQueue.refresh();
		await settle(100);

		expect(cardLabels()).toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(FEEDS.length);
	});

	it("marks a gallery tile as read when closing the reader overlay flushes readArticleIds", async () => {
		vi.mocked(recordReadingEvents).mockResolvedValueOnce({
			accepted: 1,
			duplicates: 0,
			readArticleIds: ["art-1"],
		});

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		render(ArticleReaderOverlay);
		await settle(400);

		const tile = page.getByRole("button", { name: openLabel(FIRST) });
		await expect.element(tile).toHaveAttribute("data-read", "false");

		await tile.click();
		await expect.element(page.getByTestId("reader-overlay")).toBeVisible();
		await expect
			.element(page.getByText("Gallery article body"))
			.toBeInTheDocument();

		await page.getByTestId("reader-overlay-close").click();

		await vi.waitFor(() =>
			expect(tile.element().getAttribute("data-read")).toBe("true"),
		);
	});

	it("shows Gallery mobile heading with edge padding at 393px and preserves desktop Visual Preview header", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		const mobileHeading = page.getByRole("heading", {
			level: 1,
			name: "Gallery",
		});
		await expect.element(mobileHeading).toBeInTheDocument();

		const headerEl = document.querySelector<HTMLElement>(".gallery-header");
		expect(headerEl).not.toBeNull();
		if (headerEl) {
			expect(getComputedStyle(headerEl).paddingLeft).toBe("16px");
			expect(getComputedStyle(headerEl).paddingRight).toBe("16px");
		}

		const gridEl = page.getByTestId("gallery-grid").element();
		expect(gridEl.className).toContain("px-2");
		expect(gridEl.className).toContain("md:px-0");

		await page.viewport(1024, 800);
		await settle(250);

		await expect
			.element(page.getByRole("heading", { level: 1, name: "Visual Preview" }))
			.toBeInTheDocument();
	});
});
