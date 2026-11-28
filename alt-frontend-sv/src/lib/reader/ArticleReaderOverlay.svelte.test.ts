import { beforeEach, describe, expect, it, vi } from "vitest";
import { page as testPage } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ReaderOverlayRouteState } from "./types.js";

const mockPushState = vi.fn();
const mockReplaceState = vi.fn();

let currentPageState = $state<{ reader?: ReaderOverlayRouteState }>({});
let currentPageURL = $state(new URL("http://localhost/feeds/visual-preview"));

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
	beforeNavigate: vi.fn(),
	pushState: (...args: unknown[]) => mockPushState(...args),
	replaceState: (...args: unknown[]) => mockReplaceState(...args),
}));

vi.mock("$app/state", () => ({
	page: {
		get state() {
			return currentPageState;
		},
		get url() {
			return currentPageURL;
		},
	},
}));

vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

const mockStreamSummarize = vi.fn();
vi.mock("#lib/connect/index.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		streamSummarizeWithAbortAdapter: (...args: unknown[]) =>
			mockStreamSummarize(...args),
	};
});

const mockGetFeedContent = vi.fn();
const mockGetArticleSourceURL = vi
	.fn()
	.mockResolvedValue({ url: "", title: "" });
vi.mock("#lib/api/client/articles.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		getFeedContentOnTheFlyClient: (...args: unknown[]) =>
			mockGetFeedContent(...args),
		getArticleSourceURLClient: (...args: unknown[]) =>
			mockGetArticleSourceURL(...args),
	};
});

import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import {
	getReadTodayQueue,
	QueueArticleOutcome,
	queueArticle,
	recordReadingEvents,
	unqueueArticle,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import { articlePrefetcher } from "#lib/utils/articlePrefetcher.js";
import ArticleReaderOverlay from "./ArticleReaderOverlay.svelte";
import {
	openReaderOverlay,
	resetReaderOverlaySessions,
} from "./reader-overlay.svelte.js";

describe("ArticleReaderOverlay + shallow routing", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		resetReaderOverlaySessions();
		readTodayQueue.reset();
		currentPageState = {};
		currentPageURL = new URL("http://localhost/feeds/visual-preview");

		mockPushState.mockImplementation(
			(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
				currentPageState = state;
				if (url) {
					currentPageURL = new URL(String(url), "http://localhost");
				}
			},
		);
		mockReplaceState.mockImplementation(
			(url: string | URL, state: { reader?: ReaderOverlayRouteState }) => {
				currentPageState = state;
				if (url) {
					currentPageURL = new URL(String(url), "http://localhost");
				}
			},
		);

		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 0,
			duplicates: 0,
			readArticleIds: [],
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockClear();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
			readArticleIds: [],
		});
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [],
			cap: 7,
			readCountSince: 0,
		});
		vi.mocked(queueArticle).mockResolvedValue({
			outcome: QueueArticleOutcome.QUEUED,
			items: [],
			cap: 7,
		});
		vi.mocked(unqueueArticle).mockResolvedValue(undefined);
		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);
	});

	it("opens immediately when articleId is absent and replaceStates to /articles/<id> once resolved", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Overlay body resolved id.</p>",
			article_id: "resolved-art-99",
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					url: "https://example.com/no-id-yet",
					title: "No ID Yet",
				},
			],
			currentIndex: 0,
		});

		expect(mockPushState).toHaveBeenCalledTimes(1);
		expect(mockPushState).toHaveBeenCalledWith(
			"",
			expect.objectContaining({
				reader: expect.objectContaining({
					surface: ReadingSurface.GALLERY,
					currentIndex: 0,
				}),
			}),
		);

		await expect
			.element(testPage.getByText("Overlay body resolved id."))
			.toBeInTheDocument();

		await vi.waitFor(() => expect(mockReplaceState).toHaveBeenCalledTimes(1));
		expect(mockReplaceState).toHaveBeenCalledWith(
			"/articles/resolved-art-99",
			expect.objectContaining({
				reader: expect.objectContaining({
					article: expect.objectContaining({
						articleId: "resolved-art-99",
					}),
				}),
			}),
		);
	});

	it("ends reading session and flushes queued events when the overlay closes", async () => {
		const overlayArtId = "01938e82-7c00-7a7b-9b10-000000000173";
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Overlay reading session body.</p>",
			article_id: overlayArtId,
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.SWIPE,
			items: [
				{
					articleId: overlayArtId,
					url: "https://example.com/overlay-1",
					title: "Overlay 1",
				},
			],
			currentIndex: 0,
		});

		await expect
			.element(testPage.getByText("Overlay reading session body."))
			.toBeInTheDocument();

		expect(recordReadingEvents).not.toHaveBeenCalled();

		// Simulate browser Back / overlay close clearing page.state.reader
		currentPageState = {};

		await vi.waitFor(() =>
			expect(recordReadingEvents).toHaveBeenCalledTimes(1),
		);
		const events = vi.mocked(recordReadingEvents).mock.calls[0]?.[1] ?? [];
		expect(events.map((e) => e.event?.case)).toContain("sessionEnded");
		const sessionEnded = events.find((e) => e.event?.case === "sessionEnded")
			?.event?.value;
		expect(sessionEnded).toMatchObject({
			articleId: overlayArtId,
			surface: ReadingSurface.SWIPE,
		});
	});

	it("navigates prev/next via replaceState and calls history.back() on close button", async () => {
		const backSpy = vi.spyOn(history, "back").mockImplementation(() => {
			currentPageState = {};
		});
		mockGetFeedContent
			.mockResolvedValueOnce({
				content: "<p>First article</p>",
				article_id: "art-1",
			})
			.mockResolvedValueOnce({
				content: "<p>Second article</p>",
				article_id: "art-2",
			});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					articleId: "art-1",
					url: "https://example.com/1",
					title: "Article 1",
				},
				{
					articleId: "art-2",
					url: "https://example.com/2",
					title: "Article 2",
				},
			],
			currentIndex: 0,
		});

		expect(mockPushState).toHaveBeenCalledWith(
			"/articles/art-1",
			expect.anything(),
		);
		await expect
			.element(testPage.getByText("First article"))
			.toBeInTheDocument();

		await testPage.getByTestId("reader-next-button").click();

		expect(mockReplaceState).toHaveBeenCalledWith(
			"/articles/art-2",
			expect.objectContaining({
				reader: expect.objectContaining({
					currentIndex: 1,
				}),
			}),
		);
		await expect
			.element(testPage.getByText("Second article"))
			.toBeInTheDocument();

		await testPage.getByTestId("reader-overlay-close").click();
		expect(backSpy).toHaveBeenCalledTimes(1);

		backSpy.mockRestore();
	});

	it("sends opened once when articleId resolves, drops readArticleIds from queue, and notifies onArticlesRead", async () => {
		const resolvedOpenId = "01938e82-7c00-7a7b-9b10-000000000279";
		await readTodayQueue.refresh();
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUED,
			items: [
				{
					articleId: resolvedOpenId,
					title: "Queued From Before",
					url: "https://example.com/open-1",
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
		});
		await readTodayQueue.queue({
			articleId: resolvedOpenId,
			surface: ReadingSurface.SWIPE,
			title: "Queued From Before",
			url: "https://example.com/open-1",
		});
		expect(readTodayQueue.isQueued(resolvedOpenId)).toBe(true);

		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Body for opened + readArticleIds.</p>",
			article_id: resolvedOpenId,
		});
		vi.mocked(recordReadingEvents).mockResolvedValueOnce({
			accepted: 2,
			duplicates: 0,
			readArticleIds: [resolvedOpenId],
		});

		const onArticlesRead = vi.fn();
		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					url: "https://example.com/open-1",
					title: "Queued From Before",
				},
			],
			currentIndex: 0,
			onArticlesRead,
		});

		await expect
			.element(testPage.getByText("Body for opened + readArticleIds."))
			.toBeInTheDocument();

		// Close overlay -> flushes opened + sessionEnded
		currentPageState = {};

		await vi.waitFor(() =>
			expect(recordReadingEvents).toHaveBeenCalledTimes(1),
		);
		const events = vi.mocked(recordReadingEvents).mock.calls[0]?.[1] ?? [];
		const openedEvents = events.filter((e) => e.event?.case === "opened");
		expect(openedEvents).toHaveLength(1);
		expect(openedEvents[0]?.event?.value).toMatchObject({
			articleId: resolvedOpenId,
			surface: ReadingSurface.GALLERY,
		});

		await vi.waitFor(() =>
			expect(onArticlesRead).toHaveBeenCalledWith([resolvedOpenId]),
		);
		expect(readTodayQueue.isQueued(resolvedOpenId)).toBe(false);
	});

	it("toggles Read today / In Read today from the reader action cluster", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Toggle queue body.</p>",
			article_id: "art-toggle-1",
		});
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUED,
			items: [
				{
					articleId: "art-toggle-1",
					title: "Toggle Article",
					url: "https://example.com/toggle-1",
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.OTHER,
			items: [
				{
					articleId: "art-toggle-1",
					url: "https://example.com/toggle-1",
					title: "Toggle Article",
				},
			],
			currentIndex: 0,
		});

		await expect
			.element(testPage.getByText("Toggle queue body."))
			.toBeInTheDocument();

		const toggleBtn = testPage.getByTestId("read-today-toggle-button");
		await expect.element(toggleBtn).toHaveAccessibleName("Read today");
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "false");

		await toggleBtn.click();

		await expect.element(toggleBtn).toHaveAccessibleName("In Read today");
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "true");
		expect(queueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: "art-toggle-1",
			surface: ReadingSurface.OTHER,
		});

		await toggleBtn.click();

		await expect.element(toggleBtn).toHaveAccessibleName("Read today");
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "false");
		expect(unqueueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: "art-toggle-1",
		});
	});

	it("fits all eight reader overlay controls in a 360px viewport without horizontal overflow and with >= 40x44 targets", async () => {
		await testPage.viewport(360, 800);
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Mobile bar layout test.</p>",
			article_id: "art-bar-1",
		});
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUED,
			items: [
				{
					articleId: "art-bar-1",
					title: "Article 1",
					url: "https://example.com/bar-1",
					sourceName: "Source",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					articleId: "art-bar-1",
					url: "https://example.com/bar-1",
					title: "Article 1",
				},
				{
					articleId: "art-bar-2",
					url: "https://example.com/bar-2",
					title: "Article 2",
				},
			],
			currentIndex: 0,
			onMarkAsRead: vi.fn(),
		});

		await expect
			.element(testPage.getByText("Mobile bar layout test."))
			.toBeInTheDocument();

		const bar = document.querySelector<HTMLElement>("header.overlay-bar");
		expect(bar).not.toBeNull();
		if (bar) {
			expect(bar.scrollWidth).toBeLessThanOrEqual(bar.clientWidth);
		}

		const controls = [
			testPage.getByTestId("reader-overlay-close").element(),
			testPage.getByTestId("reader-prev-button").element(),
			testPage.getByTestId("reader-next-button").element(),
			testPage.getByTestId("reader-mark-read-button").element(),
			testPage.getByTestId("read-today-toggle-button").element(),
			testPage.getByTestId("fetch-button").element(),
			testPage.getByTestId("summarize-button").element(),
			testPage.getByRole("link", { name: "Open original" }).element(),
		];

		for (const el of controls) {
			const rect = el.getBoundingClientRect();
			expect(rect.width).toBeGreaterThanOrEqual(40);
			expect(rect.height).toBeGreaterThanOrEqual(44);
			expect(rect.right).toBeLessThanOrEqual(360);
		}

		const toggleBtn = testPage.getByTestId("read-today-toggle-button");
		await toggleBtn.click();
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "true");
		expect(getComputedStyle(toggleBtn.element()).borderBottomWidth).toBe("2px");
	});

	it("shows full-queue error without calling queueArticle when remainingSlots is 0", async () => {
		const targetId = "01938e82-7c00-7a7b-9b10-000000000555";
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: Array.from({ length: 6 }, (_, i) => ({
				articleId: `01938e82-7c00-7a7b-9b10-00000000060${i}`,
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
		await readTodayQueue.refresh();
		readTodayQueue.setPendingQueueCount(1);
		expect(readTodayQueue.remainingSlots).toBe(0);

		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Full queue test body.</p>",
			article_id: targetId,
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					articleId: targetId,
					url: "https://example.com/full-queue-article",
					title: "Unqueued Article",
				},
			],
			currentIndex: 0,
		});

		await expect
			.element(testPage.getByText("Full queue test body."))
			.toBeInTheDocument();

		const toggleBtn = testPage.getByTestId("read-today-toggle-button");
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "false");
		await toggleBtn.click();

		expect(queueArticle).not.toHaveBeenCalled();
		await expect
			.element(
				testPage.getByText("Read today is full (7). Read or remove one first."),
			)
			.toBeInTheDocument();
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "false");
	});

	it("treats UNSPECIFIED outcome from queueArticle as a failure in reader toggle", async () => {
		const targetId = "01938e82-7c00-7a7b-9b10-000000000556";
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.UNSPECIFIED,
			items: [],
			cap: 7,
		});
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Unspecified outcome body.</p>",
			article_id: targetId,
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.GALLERY,
			items: [
				{
					articleId: targetId,
					url: "https://example.com/unspecified-article",
					title: "Unspecified Article",
				},
			],
			currentIndex: 0,
		});

		await expect
			.element(testPage.getByText("Unspecified outcome body."))
			.toBeInTheDocument();

		const toggleBtn = testPage.getByTestId("read-today-toggle-button");
		await toggleBtn.click();

		await expect
			.element(testPage.getByText("Could not add to Read today."))
			.toBeInTheDocument();
		await expect.element(toggleBtn).toHaveAttribute("aria-pressed", "false");
	});

	it("renders the editorial rail with excerpt on wide viewports and omits it on narrow viewports", async () => {
		mockGetFeedContent.mockResolvedValue({
			content: "<p>Wide desktop article body.</p>",
			article_id: "art-rail-1",
		});

		await testPage.viewport(1440, 900);
		const { unmount } = render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.OTHER,
			items: [
				{
					articleId: "art-rail-1",
					url: "https://example.com/rail-article",
					title: "Rail Article",
					excerpt: "Editorial excerpt summary for the rail.",
				},
			],
			currentIndex: 0,
		});

		const rail = testPage.getByTestId("article-rail");
		await expect.element(rail).toBeVisible();
		await expect
			.element(rail.getByText("Editorial excerpt summary for the rail."))
			.toBeVisible();
		await expect.element(rail.getByText("example.com")).toBeVisible();

		await testPage.viewport(390, 844);
		await expect.element(rail).not.toBeInTheDocument();

		currentPageState = {};
		unmount();
	});

	it("triggers prefetch for the next 2 articles when opened on a list", async () => {
		const triggerSpy = vi.spyOn(articlePrefetcher, "triggerPrefetch");
		mockGetFeedContent.mockResolvedValue({
			content: "<p>First article body.</p>",
			article_id: "art-1",
		});

		const { unmount } = render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.OTHER,
			items: [
				{
					articleId: "art-1",
					url: "https://example.com/1",
					title: "First",
				},
				{
					articleId: "art-2",
					url: "https://example.com/2",
					title: "Second",
				},
				{
					articleId: "art-3",
					url: "https://example.com/3",
					title: "Third",
				},
			],
			currentIndex: 0,
		});

		await vi.waitFor(() => expect(triggerSpy).toHaveBeenCalled());
		expect(triggerSpy).toHaveBeenCalledWith(
			[
				{ normalizedUrl: "https://example.com/1" },
				{ normalizedUrl: "https://example.com/2" },
				{ normalizedUrl: "https://example.com/3" },
			],
			0,
			2,
		);

		currentPageState = {};
		unmount();
		triggerSpy.mockRestore();
	});

	it("seeds AI SUMMARY immediately from initialSummary without rendering Excerpt, resets on navigation, and clears on re-fetch", async () => {
		mockGetFeedContent
			.mockResolvedValueOnce({
				content: "<p>First seeded body.</p>",
				article_id: "art-seed-1",
			})
			.mockResolvedValueOnce({
				content: "<p>Refetched first body.</p>",
				article_id: "art-seed-1",
			})
			.mockResolvedValueOnce({
				content: "<p>Second unseeded body.</p>",
				article_id: "art-seed-2",
			});

		await testPage.viewport(1440, 900);
		const { unmount } = render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.OTHER,
			items: [
				{
					articleId: "art-seed-1",
					url: "https://example.com/seed-1",
					title: "Seeded Article",
					excerpt: null,
					fallbackDescription: null,
					initialSummary: "既存の日本語要約が最初から表示されます。",
				},
				{
					articleId: "art-seed-2",
					url: "https://example.com/seed-2",
					title: "Unseeded Article",
					excerpt: null,
					fallbackDescription: null,
					initialSummary: null,
				},
			],
			currentIndex: 0,
		});

		const rail = testPage.getByTestId("article-rail");
		await expect.element(rail).toBeVisible();
		await expect
			.element(rail.getByTestId("ai-summary"))
			.toHaveTextContent("既存の日本語要約が最初から表示されます。");
		await expect
			.element(rail.getByRole("heading", { name: /^excerpt$/i }))
			.not.toBeInTheDocument();
		await expect
			.element(rail.getByTestId("summary-speech-button"))
			.toBeVisible();
		await expect
			.element(rail.getByTestId("summarize-button"))
			.toHaveAccessibleName("Re-summarize");
		expect(mockStreamSummarize).not.toHaveBeenCalled();

		// Re-fetch clears the seeded summary and does not re-seed the stale summary
		await rail.getByTestId("fetch-button").click();
		await expect
			.element(testPage.getByText("Refetched first body."))
			.toBeInTheDocument();
		await expect
			.element(rail.getByTestId("ai-summary"))
			.not.toBeInTheDocument();

		// Navigating to an unseeded item keeps AI SUMMARY hidden
		await testPage.getByTestId("reader-next-button").click();
		await expect
			.element(testPage.getByText("Second unseeded body."))
			.toBeInTheDocument();
		await expect
			.element(rail.getByTestId("ai-summary"))
			.not.toBeInTheDocument();

		currentPageState = {};
		unmount();
	});

	it("keeps distinct RSS Excerpt alongside AI SUMMARY, but hides Excerpt when it duplicates AI SUMMARY", async () => {
		mockGetFeedContent.mockResolvedValue({
			content: "<p>Editorial rail dedupe body.</p>",
			article_id: "art-dedupe-1",
		});

		await testPage.viewport(1440, 900);
		const { unmount } = render(ArticleReaderOverlay);

		// Case 1: distinct English RSS excerpt + Japanese AI summary -> both visible
		openReaderOverlay({
			surface: ReadingSurface.OTHER,
			items: [
				{
					articleId: "art-dedupe-1",
					url: "https://example.com/dedupe-1",
					title: "Distinct Excerpt",
					excerpt: "Original English RSS excerpt from publisher.",
					initialSummary: "日本語の AI 要約テキスト。",
				},
				{
					articleId: "art-dedupe-2",
					url: "https://example.com/dedupe-2",
					title: "Duplicate Excerpt",
					excerpt: "同じ日本語要約テキスト。\n  二文目です。",
					initialSummary: "同じ日本語要約テキスト。 二文目です。",
				},
			],
			currentIndex: 0,
		});

		const rail = testPage.getByTestId("article-rail");
		await expect.element(rail).toBeVisible();
		await expect
			.element(rail.getByRole("heading", { name: /^excerpt$/i }))
			.toBeVisible();
		await expect
			.element(rail.getByText("Original English RSS excerpt from publisher."))
			.toBeVisible();
		await expect
			.element(rail.getByTestId("ai-summary"))
			.toHaveTextContent("日本語の AI 要約テキスト。");

		// Case 2: Excerpt duplicates AI SUMMARY (modulo whitespace) -> Excerpt hidden
		await testPage.getByTestId("reader-next-button").click();
		await expect
			.element(rail.getByTestId("ai-summary"))
			.toHaveTextContent("同じ日本語要約テキスト。 二文目です。");
		await expect
			.element(rail.getByRole("heading", { name: /^excerpt$/i }))
			.not.toBeInTheDocument();

		currentPageState = {};
		unmount();
	});
});
