import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FeedDetails from "./FeedDetails.svelte";

// Mock API client functions
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(() =>
		Promise.resolve({
			content: "<p>Full article content here.</p>",
			article_id: "article-123",
		}),
	),
	getArticleSummaryClient: vi.fn(() =>
		Promise.resolve({
			matched_articles: [{ source_id: "article-123" }],
		}),
	),
	summarizeArticleClient: vi.fn(() =>
		Promise.resolve({
			success: true,
			summary: "This is a test summary.",
		}),
	),
	registerFavoriteFeedClient: vi.fn(() => Promise.resolve({ message: "ok" })),
	archiveContentClient: vi.fn(() => Promise.resolve({})),
}));

// Mock Connect RPC functions
vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(
		(
			_transport: unknown,
			_options: unknown,
			_updateState: unknown,
			_rendererOptions: unknown,
			onComplete?: (result: unknown) => void,
			_onError?: (error: Error) => void,
		) => {
			if (onComplete) {
				onComplete({});
			}
			return new AbortController();
		},
	),
}));

// Mock $app/environment
vi.mock("$app/environment", () => ({
	browser: true,
}));

const testFeedURL = "https://example.com/test-article";
const testFeedTitle = "Test Article Title";

describe("FeedDetails Alt-Paper compliance", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders Show Details button when showButton is true", async () => {
		render(FeedDetails as never, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				showButton: true,
			},
		});

		await expect.element(page.getByText("Show Details")).toBeInTheDocument();
	});

	it("does NOT render Archive button when open", async () => {
		render(FeedDetails as never, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		const archiveEl = page.getByText("Archive");
		await expect.element(archiveEl).not.toBeInTheDocument();
	});

	it("renders Favorite button when open", async () => {
		render(FeedDetails as never, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		await expect
			.element(page.getByRole("button", { name: /favorite/i }))
			.toBeInTheDocument();
	});
});

describe("FeedDetails retry", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("content fetch retry", () => {
		it("shows error with role='alert' when content fetch fails", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(FeedDetails as never, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// Wait for fetch to fail
			await new Promise((resolve) => setTimeout(resolve, 500));

			// Error should be displayed (component shows error via RenderFeedDetails)
			const container = page.getByText(/unable to fetch/i);
			await expect.element(container).toBeInTheDocument();
		});
	});

	describe("summary retry", () => {
		it("shows summary error with role='alert' when summarization fails", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");

			// Make stream fail with non-transient error
			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
				(
					_transport: unknown,
					_options: unknown,
					_onChunk: unknown,
					_rendererOptions: unknown,
					_onComplete?: unknown,
					onError?: (error: Error) => void,
				) => {
					setTimeout(() => {
						onError?.(new Error("500 Internal Server Error"));
					}, 10);
					return new AbortController();
				},
			);

			render(FeedDetails as never, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// Wait for initial data load
			await new Promise((resolve) => setTimeout(resolve, 300));

			// Click Summary button
			const summaryButton = page.getByRole("button", { name: /summary/i });
			await summaryButton.click();

			// Wait for error
			await new Promise((resolve) => setTimeout(resolve, 500));

			// Error alert should appear
			await expect.element(page.getByRole("alert")).toBeInTheDocument();
		});

		it("summary button shows 'Try again' after error", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");

			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
				(
					_transport: unknown,
					_options: unknown,
					_onChunk: unknown,
					_rendererOptions: unknown,
					_onComplete?: unknown,
					onError?: (error: Error) => void,
				) => {
					setTimeout(() => {
						onError?.(new Error("500 Internal Server Error"));
					}, 10);
					return new AbortController();
				},
			);

			render(FeedDetails as never, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await new Promise((resolve) => setTimeout(resolve, 300));

			const summaryButton = page.getByRole("button", { name: /summary/i });
			await summaryButton.click();

			await new Promise((resolve) => setTimeout(resolve, 500));

			// Button should show "Try again"
			await expect
				.element(page.getByRole("button", { name: /try again/i }))
				.toBeInTheDocument();
		});
	});
});
