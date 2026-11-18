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
import { recordReadingEvents } from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";
import ArticleReaderOverlay from "./ArticleReaderOverlay.svelte";
import {
	openReaderOverlay,
	resetReaderOverlaySessions,
} from "./reader-overlay.svelte.js";

describe("ArticleReaderOverlay + shallow routing", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		resetReaderOverlaySessions();
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
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockClear();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
		});
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
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Overlay reading session body.</p>",
			article_id: "art-overlay-1",
		});

		render(ArticleReaderOverlay);

		openReaderOverlay({
			surface: ReadingSurface.SWIPE,
			items: [
				{
					articleId: "art-overlay-1",
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
			articleId: "art-overlay-1",
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
});
