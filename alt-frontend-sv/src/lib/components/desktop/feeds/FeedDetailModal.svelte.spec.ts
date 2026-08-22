/**
 * FeedDetailModal content-state tests.
 *
 * The desktop modal is where ADR-000581's infinite auto-fetch `$effect` was
 * first found and closed, so every case here is written to fail loudly rather
 * than hang: each one pins the number of requests the modal sends on its own.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { page as testPage } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type { RenderFeed } from "$lib/schema/feed";

const mockGetFeedContent = vi.fn();
vi.mock("$lib/api/client/articles", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
}));

// Stubbed rather than exercised: the prefetcher is a module singleton whose
// cache would leak between cases, and its own behaviour has its own tests.
vi.mock("$lib/utils/articlePrefetcher", () => ({
	articlePrefetcher: {
		getCachedContent: () => null,
		getCachedArticleId: () => null,
		triggerPrefetch: vi.fn(),
	},
}));

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(() => new AbortController()),
}));

import FeedDetailModal from "./FeedDetailModal.svelte";

const FEED: RenderFeed = {
	id: "feed-1",
	title: "Test Article Title",
	description: "The RSS description, which must survive every failure.",
	link: "https://example.com/test-article",
	published: "2026-08-01T10:00:00Z",
	created_at: "2026-08-01T09:00:00Z",
	author: "Test Author",
	publishedAtFormatted: "Aug 1, 2026",
	mergedTagsLabel: "Test / Svelte",
	normalizedUrl: "https://example.com/test-article",
	excerpt: "An excerpt.",
};

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

function renderModal() {
	return render(FeedDetailModal, {
		props: { open: true, feed: FEED, onOpenChange: () => {} },
	});
}

describe("FeedDetailModal content states", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("says what it is doing while the body is in flight", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Fetching the full article");
		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.not.toBeInTheDocument();
		// The RSS body is on screen underneath the wait.
		await expect
			.element(testPage.getByTestId("article-fallback-summary"))
			.toHaveTextContent("The RSS description");
	});

	it("offers both remedies, and never the upstream prose, when it is terminal", async () => {
		mockGetFeedContent.mockRejectedValue(connectError(Code.Unavailable));

		renderModal();

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
			.toHaveAttribute("href", FEED.link);
		await expect
			.element(testPage.getByTestId("article-fallback-summary"))
			.toBeInTheDocument();
	});

	it("states the shared wording for an empty body and asks exactly once", async () => {
		// ADR-000581's loop lived exactly here.
		mockGetFeedContent.mockResolvedValue({ content: "", article_id: "" });

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toHaveTextContent("Source content unavailable.");
		await new Promise((r) => setTimeout(r, 600));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	it("does NOT auto-retry Code.Unavailable — ADR-000959 rejected exactly that", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.Unavailable, { "Retry-After": "0" }),
		);

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	it("retries an unstamped ResourceExhausted exactly once, announcing the wait", async () => {
		mockGetFeedContent
			.mockRejectedValueOnce(
				connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
			)
			.mockResolvedValue({
				content: "<p>Arrived on the second attempt.</p>",
				article_id: "a1",
			});

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Retrying");
		await expect
			.element(testPage.getByText("Arrived on the second attempt."))
			.toBeInTheDocument();
		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
	});

	it("does NOT retry a ResourceExhausted stamped as the publisher's own 429", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.ResourceExhausted, {
				"Retry-After": "0",
				"X-Alt-Failure-Scope": "host",
			}),
		);

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});
});
