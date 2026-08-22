import { Code, ConnectError } from "@connectrpc/connect";
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

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

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

describe("ArticleDetailPanel content states", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		summarizerOverride = {};
	});

	it("says what it is doing while the body is in flight", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		renderPanel();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Fetching the full article");
		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.not.toBeInTheDocument();
	});

	it("offers both remedies, and never the upstream prose, when it is terminal", async () => {
		mockGetFeedContent.mockRejectedValue(connectError(Code.Unavailable));

		renderPanel();

		const failed = testPage.getByTestId("article-content-failed");
		await expect
			.element(failed)
			.toHaveTextContent(
				"Source content is temporarily unavailable. Please try again shortly.",
			);
		await expect.element(failed).not.toHaveTextContent("circuit breaker");
		await expect
			.element(testPage.getByTestId("retry-content"))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByTestId("read-original-link"))
			.toHaveAttribute("href", ARTICLE.link);
	});

	it("treats an empty body as a stated failure rather than a blank panel", async () => {
		mockGetFeedContent.mockResolvedValue({
			content: "",
			article_id: "",
			og_image_url: "",
			og_image_proxy_url: "",
		});

		renderPanel();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toHaveTextContent("Source content unavailable.");
	});

	it("retries an unstamped ResourceExhausted once and never a Code.Unavailable", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
		);
		const first = renderPanel();
		await new Promise((r) => setTimeout(r, 1200));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
		first.unmount();

		mockGetFeedContent.mockClear();
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.Unavailable, { "Retry-After": "0" }),
		);
		renderPanel();
		await new Promise((r) => setTimeout(r, 1200));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});
});
