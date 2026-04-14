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
let summarizerOverride: Partial<{
	summary: string | null;
	isSummarizing: boolean;
	summaryError: string | null;
	buttonState: "idle" | "loading" | "error" | "success";
}> = {};
vi.mock("$lib/hooks/useSummarize.svelte", () => ({
	useSummarize: () => ({
		summary: null,
		isSummarizing: false,
		summaryError: null,
		buttonState: "idle" as const,
		summarize: vi.fn(),
		abort: vi.fn(),
		reset: mockSummarizerReset,
		...summarizerOverride,
	}),
}));

import Page from "./+page.svelte";

describe("Article page fetch button", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		summarizerOverride = {};
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

describe("Article page Alt-Paper mobile layout", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		summarizerOverride = {};
	});

	it("renders editorial masthead with page-kicker role and article title", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		render(Page as never);

		const masthead = testPage.getByTestId("article-masthead");
		await expect.element(masthead).toBeInTheDocument();
		await expect.element(masthead).toHaveAttribute("data-role", "page-kicker");
		await expect.element(masthead).toHaveTextContent("Test Article");
	});

	it("uses Alt-Paper surface tokens without rounded-lg/bg-white violations", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		const { container } = render(Page as never);

		await expect
			.element(testPage.getByTestId("article-content-surface"))
			.toBeInTheDocument();

		const offenders = container.querySelectorAll(
			".rounded-lg, .rounded-xl, .rounded-2xl, .bg-white",
		);
		expect(offenders.length).toBe(0);
	});

	it("renders AI SUMMARY kicker when summary is present", async () => {
		summarizerOverride = {
			summary: "Condensed bullet list",
			buttonState: "success",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		render(Page as never);

		const summary = testPage.getByTestId("ai-summary");
		await expect.element(summary).toBeInTheDocument();
		await expect.element(summary).toHaveTextContent("AI SUMMARY");
		await expect.element(summary).toHaveTextContent("Condensed bullet list");
	});

	it("exposes icon-only actions with accessible labels on mobile", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		render(Page as never);

		await expect
			.element(testPage.getByRole("button", { name: /back to home/i }))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByTestId("fetch-button"))
			.toHaveAttribute("aria-label");
		await expect
			.element(testPage.getByTestId("summarize-button"))
			.toHaveAttribute("aria-label");
		await expect
			.element(testPage.getByRole("link", { name: /open original/i }))
			.toBeInTheDocument();
	});
});
