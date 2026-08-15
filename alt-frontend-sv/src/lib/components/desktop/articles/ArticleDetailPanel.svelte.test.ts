import { page as testPage } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockGetFeedContent = vi.fn();
vi.mock("$lib/api/client/articles", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
}));

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
		reset: vi.fn(),
		...summarizerOverride,
	}),
}));

import ArticleDetailPanel from "./ArticleDetailPanel.svelte";

const ARTICLE = {
	id: "article-1",
	title: "Test Article",
	link: "https://example.com/article",
	publishedAt: "2026-08-01T00:00:00Z",
	feedTitle: "Example Feed",
};

function renderPanel() {
	return render(ArticleDetailPanel, {
		props: { article: ARTICLE, onClose: () => {} },
	});
}

describe("ArticleDetailPanel summary interruption", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		summarizerOverride = {};
		mockGetFeedContent.mockResolvedValue({
			content: "<p>Article body</p>",
			article_id: "article-1",
			og_image_url: "",
			og_image_proxy_url: "",
		});
	});

	it("flags an interrupted stream alongside the partial summary", async () => {
		summarizerOverride = {
			summary: "Half a sentence before the stream",
			summaryError: "[unknown] missing EndStreamResponse",
			buttonState: "error",
		};

		renderPanel();

		await expect
			.element(testPage.getByTestId("ai-summary"))
			.toHaveTextContent("Half a sentence before the stream");
		await expect
			.element(testPage.getByTestId("summary-interrupted"))
			.toHaveTextContent("Stream interrupted. Summary may be incomplete.");
	});

	it("shows the summarize error on its own when no text arrived", async () => {
		summarizerOverride = {
			summary: null,
			summaryError: "Failed to generate summary",
			buttonState: "error",
		};

		renderPanel();

		await expect
			.element(testPage.getByTestId("summary-error"))
			.toHaveTextContent("Failed to generate summary");
	});

	it("shows no interruption notice for a summary that completed", async () => {
		summarizerOverride = {
			summary: "Condensed bullet list",
			summaryError: null,
			buttonState: "success",
		};

		const { container } = renderPanel();

		await expect
			.element(testPage.getByTestId("ai-summary"))
			.toBeInTheDocument();
		expect(
			container.querySelector('[data-testid="summary-interrupted"]'),
		).toBeNull();
		expect(container.querySelector('[data-testid="summary-error"]')).toBeNull();
	});
});
