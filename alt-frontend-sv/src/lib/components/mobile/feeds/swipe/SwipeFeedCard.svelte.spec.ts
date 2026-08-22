/**
 * SwipeFeedCard Component Tests
 *
 * Tests for the swipeable feed card component using vitest-browser-svelte.
 * Tests interaction patterns, accessibility, and state management.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import type { RenderFeed } from "$lib/schema/feed";
import SwipeFeedCard from "./SwipeFeedCard.svelte";

// Mock fixture for testing
const mockFeed: RenderFeed = {
	id: "feed-test-1",
	title: "Test Article Title",
	description: "This is a test description for the article.",
	link: "https://example.com/test-article",
	published: "2025-01-15T10:00:00Z",
	created_at: "2025-01-15T09:00:00Z",
	author: "Test Author",
	publishedAtFormatted: "Jan 15, 2025",
	mergedTagsLabel: "Test / Svelte",
	normalizedUrl: "https://example.com/test-article",
	excerpt: "This is a test excerpt for the article content.",
};

// Mock API client functions
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(() =>
		Promise.resolve({
			content: "<p>Full article content here.</p>",
			article_id: "article-123",
		}),
	),
	summarizeArticleClient: vi.fn(() =>
		Promise.resolve({
			success: true,
			summary: "This is a test summary.",
		}),
	),
	registerFavoriteFeedClient: vi.fn(() => Promise.resolve({ message: "ok" })),
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
			// Simulate immediate completion
			if (onComplete) {
				onComplete({
					hasReceivedData: true,
					articleId: "article-123",
					chunkCount: 1,
					totalLength: 20,
					wasCached: false,
				});
			}
			const controller = new AbortController();
			return controller;
		},
	),
}));

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

describe("SwipeFeedCard", () => {
	const defaultProps = {
		feed: mockFeed,
		statusMessage: null,
		onDismiss: vi.fn(),
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("rendering", () => {
		it("renders the swipe card with feed title", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("heading", { name: mockFeed.title }))
				.toBeInTheDocument();
		});

		it("renders the swipe card container", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect.element(page.getByTestId("swipe-card")).toBeInTheDocument();
		});

		it("renders the action footer with buttons", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("action-footer"))
				.toBeInTheDocument();
		});

		it("renders Article button", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("renders Summary button", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /summary/i }))
				.toBeInTheDocument();
		});

		it("renders the keep stamp outside the footer, leaving 2 reading actions", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect.element(page.getByTestId("keep-stamp")).toBeInTheDocument();

			const footer = document.querySelector('[data-testid="action-footer"]');
			expect(footer?.querySelector('[data-testid="keep-stamp"]')).toBeNull();
			expect(footer?.querySelectorAll("button")).toHaveLength(2);
		});
	});

	describe("accessibility", () => {
		it("has correct aria-label for external link", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("link", { name: /open article/i }))
				.toBeInTheDocument();
		});

		it("sets aria-busy when component is busy", async () => {
			render(SwipeFeedCard, {
				props: {
					...defaultProps,
					isBusy: true,
				},
			});

			const card = page.getByTestId("swipe-card");
			await expect.element(card).toHaveAttribute("aria-busy", "true");
		});

		it("external link has correct rel attributes for security", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const link = page.getByRole("link", { name: /open article/i });
			await expect.element(link).toHaveAttribute("target", "_blank");
			await expect.element(link).toHaveAttribute("rel", "noopener noreferrer");
		});
	});

	describe("feed description", () => {
		it("displays feed description when available", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByText(mockFeed.description))
				.toBeInTheDocument();
		});

		it("displays published date when available", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			// The component formats the date, so we check for presence of date
			const card = page.getByTestId("swipe-card");
			await expect.element(card).toBeInTheDocument();
		});
	});

	describe("button interactions", () => {
		it("Article button is enabled initially", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const articleButton = page.getByRole("button", { name: /article/i });
			await expect.element(articleButton).not.toBeDisabled();
		});

		it("Summary button is enabled initially", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const summaryButton = page.getByRole("button", { name: /summary/i });
			await expect.element(summaryButton).not.toBeDisabled();
		});
	});

	describe("content caching", () => {
		it("uses cached content when getCachedContent returns value", async () => {
			const cachedContent = "<p>Cached article content</p>";
			const getCachedContent = vi.fn(() => cachedContent);

			render(SwipeFeedCard, {
				props: {
					...defaultProps,
					getCachedContent,
				},
			});

			// getCachedContent should be called with the normalizedUrl
			expect(getCachedContent).toHaveBeenCalledWith(mockFeed.normalizedUrl);
		});

		it("calls onArticleIdResolved when cached articleId exists", async () => {
			const getCachedContent = vi.fn(() => "<p>Content</p>");
			const getCachedArticleId = vi.fn(() => "cached-article-id");
			const onArticleIdResolved = vi.fn();

			render(SwipeFeedCard, {
				props: {
					...defaultProps,
					getCachedContent,
					getCachedArticleId,
					onArticleIdResolved,
				},
			});

			// Wait for onMount to execute
			await new Promise((resolve) => setTimeout(resolve, 50));

			expect(getCachedArticleId).toHaveBeenCalledWith(mockFeed.normalizedUrl);
			expect(onArticleIdResolved).toHaveBeenCalledWith(
				mockFeed.link,
				"cached-article-id",
			);
		});
	});

	describe("initial article content", () => {
		it("uses initialArticleContent when provided", async () => {
			const initialContent = "<p>Pre-loaded article content</p>";

			render(SwipeFeedCard, {
				props: {
					...defaultProps,
					initialArticleContent: initialContent,
				},
			});

			// The component should have the content ready without fetching
			await expect.element(page.getByTestId("swipe-card")).toBeInTheDocument();
		});
	});

	describe("summary abort on destroy", () => {
		it("aborts summary stream when component is destroyed", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");
			const mockAbortController = new AbortController();
			const abortSpy = vi.spyOn(mockAbortController, "abort");

			// Don't call onComplete so stream stays "in-flight"
			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(() => {
				return mockAbortController;
			});

			const { unmount } = render(SwipeFeedCard, {
				props: defaultProps,
			});

			// Click Summary button to start streaming
			const summaryButton = page.getByRole("button", { name: /summary/i });
			await summaryButton.click();
			await new Promise((resolve) => setTimeout(resolve, 50));

			// Destroy component (simulates swiping away)
			unmount();
			await new Promise((resolve) => setTimeout(resolve, 50));

			expect(abortSpy).toHaveBeenCalled();
		});

		it("does not error when destroyed without active summary", async () => {
			const { unmount } = render(SwipeFeedCard, {
				props: defaultProps,
			});

			// Destroy without ever requesting summary - should not throw
			expect(() => unmount()).not.toThrow();
		});
	});

	describe("link structure", () => {
		it("article link points to correct URL", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const link = page.getByRole("link", { name: /open article/i });
			await expect.element(link).toHaveAttribute("href", mockFeed.link);
		});
	});

	describe("article retry", () => {
		it("shows error state on Article button when content fetch fails", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			// Click the article action to trigger a fetch. Addressed by testid,
			// not by name: the background fetch fails first now, so by the time
			// the click lands the button may already read "Try again".
			const articleButton = page.getByTestId("article-action");
			await articleButton.click();

			// Wait for error state
			await new Promise((resolve) => setTimeout(resolve, 200));

			// Button should show "Try again". Addressed by testid: the panel's
			// own retry control now carries that label too, so a name-based
			// locator matches two elements.
			await expect
				.element(page.getByTestId("article-action"))
				.toHaveTextContent(/try again/i);
		});

		it("retries content fetch when error button is clicked", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			// Persistent reject/resolve rather than a queue of `Once` mocks: the
			// component also fires a background fetch from onMount, so the click
			// is not the first call and a queue is consumed in an order this test
			// does not control. Swapping the behaviour once the error state is on
			// screen makes the test independent of that ordering.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			// Click the article action
			const articleButton = page.getByTestId("article-action");
			await articleButton.click();

			// Wait for error state
			const retryButton = page.getByTestId("article-action");
			await expect.element(retryButton).toHaveTextContent(/try again/i);

			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "<p>Loaded content</p>",
				article_id: "art-1",
				og_image_url: "",
				og_image_proxy_url: "",
			});

			// Click retry
			await retryButton.click();

			// Content section should be visible
			await expect
				.element(page.getByTestId("content-section"))
				.toBeInTheDocument();
			// ...and actually hold the refetched article. The section alone is
			// not evidence of a successful retry: it is already on screen while
			// the error is showing, so without these two the assertion above
			// passes even when the retry does nothing.
			await expect
				.element(page.getByText("Loaded content"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("source-unavailable-notice"))
				.not.toBeInTheDocument();
		});

		it("shows content error with role='alert'", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const articleButton = page.getByTestId("article-action");
			await articleButton.click();

			await new Promise((resolve) => setTimeout(resolve, 200));

			await expect.element(page.getByRole("alert")).toBeInTheDocument();
		});
	});

	describe("summary retry", () => {
		// Summarization has two paths, and "summarization fails" means both of
		// them failed: when the stream errors non-transiently the component
		// falls back to the legacy summarizeArticleClient endpoint. The
		// module-level mock at the top of this file resolves that fallback, so
		// failing only the stream produces a *successful* summary — the two
		// tests below have to fail the fallback as well or they assert an error
		// surface the component was never asked to show. The fallback-succeeds
		// path is covered separately in "summary fallback" below.
		const failStream = async () => {
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
		};

		const failLegacyFallback = async () => {
			const { summarizeArticleClient } = await import("$lib/api/client");
			vi.mocked(summarizeArticleClient).mockRejectedValue(
				new Error("500 Internal Server Error"),
			);
		};

		it("shows error state on Summary button when summarization fails", async () => {
			await failStream();
			await failLegacyFallback();

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			// Click Summary button
			const summaryButton = page.getByRole("button", { name: /summary/i });
			await summaryButton.click();

			// Button should show "Try again"
			await expect
				.element(page.getByTestId("summary-action"))
				.toHaveTextContent(/try again/i);
		});

		it("shows summary error with role='alert'", async () => {
			await failStream();
			await failLegacyFallback();

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const summaryButton = page.getByRole("button", { name: /summary/i });
			await summaryButton.click();

			await expect.element(page.getByRole("alert")).toBeInTheDocument();
		});

		it("keeps the summary error inside the AI summary section", async () => {
			await failStream();
			await failLegacyFallback();

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await page.getByRole("button", { name: /summary/i }).click();

			const alert = page.getByRole("alert");
			await expect.element(alert).toBeInTheDocument();
			await expect
				.element(page.getByTestId("ai-summary-section"))
				.toBeInTheDocument();
			expect(
				document
					.querySelector('[data-testid="ai-summary-section"]')
					?.contains(alert.element()),
			).toBe(true);
		});
	});

	describe("summary fallback", () => {
		// The path that ambushed this file: a non-transient stream error is not
		// a failure on its own, the component retries via the legacy endpoint.
		// That behaviour had no coverage, which is why the two tests above could
		// silently assert against a rendered summary instead of an error.
		it("renders the legacy summary when the stream fails but the fallback succeeds", async () => {
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
			// Stated here rather than inherited from the module-level mock:
			// vi.clearAllMocks() clears recorded calls but keeps implementations,
			// so the rejection installed by "summary retry" above survives into
			// this test. Declaring the behaviour it depends on keeps it
			// independent of the order the tests happen to run in.
			const { summarizeArticleClient, getFeedContentOnTheFlyClient } =
				await import("$lib/api/client");
			vi.mocked(summarizeArticleClient).mockResolvedValue({
				success: true,
				summary: "This is a test summary.",
				article_id: "article-123",
				feed_url: mockFeed.link,
			});
			// Same reason: a rejection left behind by an earlier test would put
			// the ARTICLE button into its "Try again" state and make the
			// summary-scoped assertion below fail for an unrelated reason.
			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "<p>Full article content here.</p>",
				article_id: "article-123",
			} as never);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await page.getByRole("button", { name: /summary/i }).click();

			await expect
				.element(page.getByTestId("ai-summary-section"))
				.toBeInTheDocument();
			await expect
				.element(page.getByText("This is a test summary."))
				.toBeInTheDocument();

			// The fallback succeeded, so nothing may claim the summary failed.
			await expect.element(page.getByRole("alert")).not.toBeInTheDocument();
			await expect
				.element(page.getByTestId("summary-action"))
				.not.toHaveTextContent(/try again/i);
		});
	});

	describe("favorite button", () => {
		it("renders Favorite button", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /favorite/i }))
				.toBeInTheDocument();
		});

		it("Favorite button is enabled initially", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const favoriteButton = page.getByRole("button", { name: /favorite/i });
			await expect.element(favoriteButton).not.toBeDisabled();
		});

		it("Favorite button calls registerFavoriteFeedClient on click", async () => {
			const { registerFavoriteFeedClient } = await import("$lib/api/client");

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const favoriteButton = page.getByRole("button", { name: /favorite/i });
			await favoriteButton.click();

			// Wait for async handler
			await new Promise((resolve) => setTimeout(resolve, 50));

			expect(registerFavoriteFeedClient).toHaveBeenCalledWith(mockFeed.link);
		});

		it("Favorite button shows favorited state after successful call", async () => {
			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const favoriteButton = page.getByRole("button", { name: /favorite/i });
			await favoriteButton.click();

			// Wait for async handler to complete
			await new Promise((resolve) => setTimeout(resolve, 100));

			await expect
				.element(page.getByRole("button", { name: /favorited/i }))
				.toBeInTheDocument();
		});

		it("Favorite button is retryable after API error", async () => {
			const { registerFavoriteFeedClient } = await import("$lib/api/client");
			vi.mocked(registerFavoriteFeedClient).mockRejectedValueOnce(
				new Error("network error"),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const favoriteButton = page.getByRole("button", { name: /favorite/i });
			await favoriteButton.click();

			// Wait for async handler to complete
			await new Promise((resolve) => setTimeout(resolve, 100));

			// Button should show error state
			await expect
				.element(page.getByRole("button", { name: /failed/i }))
				.toBeInTheDocument();

			// Button should NOT be disabled (retryable)
			const errorButton = page.getByRole("button", { name: /failed/i });
			await expect.element(errorButton).not.toBeDisabled();
		});

		it("Favorite button recovers from error on retry", async () => {
			const { registerFavoriteFeedClient } = await import("$lib/api/client");
			vi.mocked(registerFavoriteFeedClient)
				.mockRejectedValueOnce(new Error("network error"))
				.mockResolvedValueOnce({ message: "ok" });

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			const favoriteButton = page.getByRole("button", { name: /favorite/i });
			await favoriteButton.click();

			// Wait for error state
			await new Promise((resolve) => setTimeout(resolve, 100));

			// Click again (retry)
			const retryButton = page.getByRole("button", { name: /failed/i });
			await retryButton.click();

			// Wait for success
			await new Promise((resolve) => setTimeout(resolve, 100));

			await expect
				.element(page.getByRole("button", { name: /favorited/i }))
				.toBeInTheDocument();
		});
	});

	describe("honest content states", () => {
		it("surfaces a failed background auto-fetch instead of swallowing it", async () => {
			// The onMount fetch used to console.error and stop. The reader was
			// then shown the RSS description with no sign that anything had been
			// attempted, let alone that it had failed.
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			// The panel that explains the failure is shut, so the footer is the
			// only place the reader can learn the body is gone: it has to say so
			// in words. A bare "Try again" does not — it names no subject, and
			// the reader never made a first attempt to repeat.
			await expect
				.element(page.getByTestId("article-action"))
				.toHaveTextContent(/article unavailable/i);
			// And it says it WITHOUT giving up the noun: this is still the one
			// control that opens the article, and it must stay findable as such.
			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("treats an empty body from the background fetch as a state, not a no-op", async () => {
			// `content: ""` is the ADR-000581 trap. It has to land somewhere
			// explicit or it reads as "still loading" forever.
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "",
				article_id: "",
			} as never);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("article-action"))
				.toHaveTextContent(/article unavailable/i);
			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("shows the pending state, not an error, while the fetch is in flight", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockImplementation(
				() => new Promise(() => {}),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await page.getByTestId("article-action").click();

			await expect
				.element(page.getByTestId("article-content-pending"))
				.toHaveTextContent("Fetching the full article");
			await expect
				.element(page.getByTestId("article-content-failed"))
				.not.toBeInTheDocument();
			// The RSS body is on screen underneath the wait, never a blank panel.
			await expect
				.element(page.getByTestId("article-fallback-summary"))
				.toBeInTheDocument();
		});

		it("states the shared wording and both remedies when it is genuinely terminal", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable),
			);

			render(SwipeFeedCard, {
				props: defaultProps,
			});

			await page.getByTestId("article-action").click();

			const notice = page.getByTestId("source-unavailable-notice");
			await expect
				.element(notice)
				.toHaveTextContent(
					"Source content is temporarily unavailable. Please try again shortly.",
				);
			// ADR-000959 §6: the upstream prose never reaches the reader.
			await expect.element(notice).not.toHaveTextContent("circuit breaker");
			await expect
				.element(page.getByTestId("read-original-link"))
				.toHaveAttribute("href", mockFeed.link);
			await expect
				.element(page.getByTestId("article-fallback-summary"))
				.toBeInTheDocument();
		});

		it("does not auto-retry Code.Unavailable (ADR-000959) but does retry an unstamped 429", async () => {
			const { getFeedContentOnTheFlyClient } = await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable, { "Retry-After": "0" }),
			);

			const unmount = render(SwipeFeedCard, { props: defaultProps });
			await new Promise((r) => setTimeout(r, 1200));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
			unmount.unmount();

			vi.mocked(getFeedContentOnTheFlyClient).mockClear();
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
			);

			render(SwipeFeedCard, { props: defaultProps });
			await new Promise((r) => setTimeout(r, 1200));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);
		});
	});
});
