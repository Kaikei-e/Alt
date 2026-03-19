import { render } from "vitest-browser-svelte";
import { page as testPage } from "vitest/browser";
import { describe, expect, it, vi, beforeEach } from "vitest";

vi.mock("$app/state", () => ({
	page: {
		params: { id: "test-123" },
		url: new URL(
			"http://localhost/articles/test-123?url=https%3A%2F%2Fexample.com%2Farticle&title=Test+Article",
		),
	},
}));

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const mockGetFeedContent = vi.fn();
vi.mock("$lib/api/client/articles", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
}));

const mockSummarizerReset = vi.fn();
vi.mock("$lib/hooks/useSummarize.svelte", () => ({
	useSummarize: () => ({
		summary: null,
		isSummarizing: false,
		summaryError: null,
		buttonState: "idle" as const,
		summarize: vi.fn(),
		abort: vi.fn(),
		reset: mockSummarizerReset,
	}),
}));

import Page from "./+page.svelte";

describe("Article page fetch button", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("shows disabled Fetching... button while loading", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		render(Page as never);

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toBeInTheDocument();
		await expect.element(button).toHaveTextContent("Fetching...");
		await expect.element(button).toBeDisabled();
	});

	it("shows Re-fetch button after successful fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article content</p>",
			article_id: "a1",
		});

		render(Page as never);

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");
	});

	it("shows destructive Try again button on fetch error", async () => {
		mockGetFeedContent.mockRejectedValueOnce(new Error("Network error"));

		render(Page as never);

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Try again");
	});

	it("calls getFeedContentOnTheFlyClient with forceRefresh on re-fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Content</p>",
			article_id: "a1",
		});
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>New content</p>",
			article_id: "a1",
		});

		render(Page as never);

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");

		await button.click();

		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
		expect(mockGetFeedContent).toHaveBeenLastCalledWith(
			"https://example.com/article",
			{ forceRefresh: true },
		);
	});

	it("resets summarizer on re-fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Content</p>",
			article_id: "a1",
		});
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>New content</p>",
			article_id: "a1",
		});

		render(Page as never);

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");

		await button.click();

		expect(mockSummarizerReset).toHaveBeenCalled();
	});
});
