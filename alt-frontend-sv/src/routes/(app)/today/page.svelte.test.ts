import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { ReaderOverlayRouteState } from "#lib/reader/types.js";

let currentPageState = $state<{ reader?: ReaderOverlayRouteState }>({});
let currentPageURL = $state(new URL("http://localhost/today"));

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

vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });
vi.mock("#lib/api/client/articles.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		getFeedContentOnTheFlyClient: vi.fn().mockResolvedValue({
			content: "<p>Queued article content.</p>",
			article_id: "art-1",
		}),
		getArticleSourceURLClient: vi
			.fn()
			.mockResolvedValue({ url: "", title: "" }),
	};
});

import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import {
	getReadTodayQueue,
	recordReadingEvents,
	unqueueArticle,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import ArticleReaderOverlay from "#lib/reader/ArticleReaderOverlay.svelte";
import { resetReaderOverlaySessions } from "#lib/reader/reader-overlay.svelte.js";
import { flushReadingEvents } from "#lib/reading/instrumentation.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import Page from "./+page.svelte";

async function settle(ms = 120) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/today page", () => {
	beforeEach(async () => {
		resetReaderOverlaySessions();
		readTodayQueue.reset();
		currentPageState = {};
		currentPageURL = new URL("http://localhost/today");
		mockPushState.mockClear();
		mockReplaceState.mockClear();

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

		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);
		vi.mocked(getReadTodayQueue).mockReset();
		vi.mocked(unqueueArticle).mockReset();
		vi.mocked(unqueueArticle).mockResolvedValue(undefined);
	});

	it("renders heading Today, Read today count, queue rows, and Read today: N when > 0", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [
				{
					articleId: "art-1",
					title: "First Queued Story",
					url: "https://news.example.com/first",
					sourceName: "News Daily",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 41 * 3600_000 + 1800_000),
				},
				{
					articleId: "art-2",
					title: "Second Queued Story",
					url: "https://tech.example.com/second",
					sourceName: "Tech Weekly",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 25 * 60_000),
				},
			],
			cap: 7,
			readCountSince: 4,
		});

		render(Page);
		await settle();

		await expect
			.element(page.getByRole("heading", { level: 1, name: "Today" }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("heading", { level: 2, name: /Read today/i }))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("today-queue-count"))
			.toHaveTextContent("2 / 7");

		await expect.element(page.getByText("News Daily")).toBeInTheDocument();
		await expect
			.element(page.getByText("First Queued Story"))
			.toBeInTheDocument();
		await expect.element(page.getByText("41h left")).toBeInTheDocument();

		await expect.element(page.getByText("Tech Weekly")).toBeInTheDocument();
		await expect
			.element(page.getByText("Second Queued Story"))
			.toBeInTheDocument();
		await expect.element(page.getByText("under 1h left")).toBeInTheDocument();

		await expect
			.element(page.getByTestId("today-read-count"))
			.toHaveTextContent("Read today: 4");
	});

	it("hides Read today: N when readCountSince is 0 and shows calm empty state with Swipe and Gallery links", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [],
			cap: 7,
			readCountSince: 0,
		});

		render(Page);
		await settle();

		await expect.element(page.getByText("Nothing queued.")).toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: "Swipe" }))
			.toHaveAttribute("href", "/feeds/swipe/visual-preview");
		await expect
			.element(page.getByRole("link", { name: "Gallery" }))
			.toHaveAttribute("href", "/feeds/visual-preview");
		await expect
			.element(page.getByTestId("today-read-count"))
			.not.toBeInTheDocument();
	});

	it("opens the reader overlay with queue items as prev/next context on tap", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [
				{
					articleId: "art-1",
					title: "First Queued Story",
					url: "https://news.example.com/first",
					sourceName: "News Daily",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 10 * 3600_000),
				},
				{
					articleId: "art-2",
					title: "Second Queued Story",
					url: "https://tech.example.com/second",
					sourceName: "Tech Weekly",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 20 * 3600_000),
				},
			],
			cap: 7,
			readCountSince: 0,
		});

		render(Page);
		render(ArticleReaderOverlay);
		await settle();

		await page.getByText("First Queued Story").click();

		const overlay = page.getByTestId("reader-overlay");
		await expect.element(overlay).toBeVisible();
		expect(currentPageURL.pathname).toBe("/articles/art-1");
		await expect.element(page.getByTestId("reader-next-button")).toBeEnabled();
		await expect.element(page.getByTestId("reader-prev-button")).toBeDisabled();
	});

	it("removes an item when Remove is clicked", async () => {
		vi.mocked(getReadTodayQueue).mockResolvedValue({
			items: [
				{
					articleId: "art-1",
					title: "First Queued Story",
					url: "https://news.example.com/first",
					sourceName: "News Daily",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 10 * 3600_000),
				},
			],
			cap: 7,
			readCountSince: 0,
		});

		render(Page);
		await settle();

		await page
			.getByRole("button", { name: /Remove First Queued Story/i })
			.click();

		expect(unqueueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: "art-1",
		});
		await expect.element(page.getByText("Nothing queued.")).toBeInTheDocument();
	});

	it("shows loading and error states and retries when Retry is clicked", async () => {
		vi.mocked(getReadTodayQueue)
			.mockRejectedValueOnce(new Error("Service unavailable"))
			.mockResolvedValueOnce({
				items: [
					{
						articleId: "art-recovered",
						title: "Recovered Story",
						url: "https://example.com/recovered",
						sourceName: "Example",
						imageUrl: "",
						queuedAt: new Date(),
						expiresAt: new Date(Date.now() + 12 * 3600_000),
					},
				],
				cap: 7,
				readCountSince: 1,
			});

		render(Page);
		await settle();

		await expect.element(page.getByTestId("today-error")).toBeInTheDocument();
		const retryBtn = page.getByRole("button", { name: "Retry" });
		await expect.element(retryBtn).toBeInTheDocument();

		await retryBtn.click();
		await settle();

		await expect.element(page.getByText("Recovered Story")).toBeInTheDocument();
		await expect
			.element(page.getByTestId("today-read-count"))
			.toHaveTextContent("Read today: 1");
	});
});
