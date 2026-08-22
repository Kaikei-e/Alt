/**
 * VisualPreviewCard Component Tests
 *
 * Tests for the visual preview swipe card with thumbnail images.
 * Uses vitest-browser-svelte for component testing.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { page } from "@vitest/browser/context";
import { flushSync, tick } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { RenderFeed } from "$lib/schema/feed";
import VisualPreviewCard from "./VisualPreviewCard.svelte";

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

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

describe("VisualPreviewCard", () => {
	const defaultProps = {
		feed: mockFeed,
		statusMessage: null,
		onDismiss: vi.fn(),
		thumbnailUrl: "https://cdn.example.com/hero.jpg",
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("rendering", () => {
		it("renders the visual preview card with feed title", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("heading", { name: mockFeed.title }))
				.toBeInTheDocument();
		});

		it("renders the swipe card container", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("visual-preview-card"))
				.toBeInTheDocument();
		});

		it("renders the action footer with buttons", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("action-footer"))
				.toBeInTheDocument();
		});

		it("renders Article button", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("renders Summary button", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /summary/i }))
				.toBeInTheDocument();
		});
	});

	describe("keep stamp", () => {
		it("renders the keep stamp on the thumbnail, not in the footer", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect.element(page.getByTestId("keep-stamp")).toBeInTheDocument();

			const footer = document.querySelector('[data-testid="action-footer"]');
			expect(footer?.querySelector('[data-testid="keep-stamp"]')).toBeNull();
			expect(footer?.querySelectorAll("button")).toHaveLength(2);
		});

		it("marks the feed as favorite when stamped", async () => {
			const { registerFavoriteFeedClient } = await import("$lib/api/client");
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			// Native click: the swipe action's setPointerCapture retargets
			// CDP pointer sequences to the card in this environment.
			(page.getByTestId("keep-stamp").element() as HTMLElement).click();

			expect(registerFavoriteFeedClient).toHaveBeenCalledWith(mockFeed.link);
			await expect
				.element(page.getByTestId("keep-stamp"))
				.toHaveAttribute("aria-pressed", "true");
		});
	});

	describe("thumbnail rendering", () => {
		it("renders thumbnail image when URL is provided", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("thumbnail-image"))
				.toBeInTheDocument();
		});

		it("renders fallback gradient when thumbnailUrl is null", async () => {
			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					thumbnailUrl: null,
				},
			});

			await expect
				.element(page.getByTestId("thumbnail-fallback"))
				.toBeInTheDocument();
		});

		it("thumbnail image has correct src", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect
				.element(img)
				.toHaveAttribute("src", defaultProps.thumbnailUrl);
		});

		it("thumbnail image has lazy loading", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("loading", "lazy");
		});
	});

	describe("accessibility", () => {
		it("has correct aria-label for external link", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("link", { name: /open article/i }))
				.toBeInTheDocument();
		});

		it("sets aria-busy when component is busy", async () => {
			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					isBusy: true,
				},
			});

			const card = page.getByTestId("visual-preview-card");
			await expect.element(card).toHaveAttribute("aria-busy", "true");
		});
	});

	describe("feed info", () => {
		it("displays feed description", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByText(mockFeed.description))
				.toBeInTheDocument();
		});

		it("displays 'Swipe to mark as read' text", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByText("Swipe to mark as read"))
				.toBeInTheDocument();
		});
	});

	describe("content caching", () => {
		it("uses cached content when getCachedContent returns value", async () => {
			const getCachedContent = vi.fn(() => "<p>Cached</p>");

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					getCachedContent,
				},
			});

			expect(getCachedContent).toHaveBeenCalledWith(mockFeed.normalizedUrl);
		});
	});

	describe("expanding the article panel", () => {
		// The panel used to be opened only after the fetch settled, so its
		// loading state could not be reached on a first tap: the reader waited
		// on a disabled button and then got a panel that was already in its
		// final state. When that state was the fallback, the card looked
		// permanently broken the instant it appeared, with no sign anything had
		// been attempted — which is how a few seconds of backoff read as a dead
		// article.
		it("opens on its loading state instead of after the fetch settles", async () => {
			let settle: (value: { content: string; article_id: string }) => void =
				() => {};
			vi.mocked(getFeedContentOnTheFlyClient).mockImplementation(
				() =>
					new Promise((resolve) => {
						settle = resolve as typeof settle;
					}),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await page.getByRole("button", { name: /^article$/i }).click();

			await expect
				.element(page.getByTestId("content-section"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("article-content-pending"))
				.toHaveTextContent("Fetching the full article");
			// A request still in flight is not a verdict: the failed markup must
			// not be on screen at the same time.
			await expect
				.element(page.getByTestId("article-content-failed"))
				.not.toBeInTheDocument();

			settle({ content: "<p>Full body.</p>", article_id: "article-123" });

			await expect.element(page.getByText("Full body.")).toBeInTheDocument();
		});
	});

	// These exercise the click-driven async error path in handleToggleContent.
	// They were skipped after hitting Svelte 5 `track_reactivity_loss`: the
	// component ran `getFeedContentOnTheFlyClient` through a detached
	// `.then().catch()` chain, so the post-rejection state update was not
	// flushed in the browser runner. The handler awaits inside the handler now,
	// and the panel opens before the fetch rather than after it, so both the
	// notice and the summary are reachable from a first tap.
	describe("article content fallback when source fetch fails", () => {
		it("shows 'Source content unavailable' notice and full summary when Article fetch errors", async () => {
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("[unavailable] HTTP 500"),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			const articleBtn = page.getByRole("button", { name: /^article$/i });
			await articleBtn.click();
			await tick();
			await new Promise((r) => setTimeout(r, 50));
			flushSync();
			await tick();

			await expect
				.element(page.getByTestId("source-unavailable-notice"))
				.toBeInTheDocument();

			await expect
				.element(page.getByTestId("article-fallback-summary"))
				.toBeInTheDocument();
		});

		it("shows 'Source content unavailable' notice when Article fetch returns no content", async () => {
			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "",
				article_id: "",
			} as never);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			const articleBtn = page.getByRole("button", { name: /^article$/i });
			await articleBtn.click();
			await tick();
			await new Promise((r) => setTimeout(r, 50));
			flushSync();
			await tick();

			await expect
				.element(page.getByTestId("source-unavailable-notice"))
				.toBeInTheDocument();
		});

		it("states the transient wording for a retryable failure instead of a hardcoded literal", async () => {
			// Every surface used to hardcode its own sentence, so the same
			// condition read three different ways. The wording now comes from
			// articleContentErrorMessage() alone, which also guarantees the
			// upstream prose ("...circuit breaker") never reaches the reader.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await page.getByRole("button", { name: /^article$/i }).click();

			const notice = page.getByTestId("source-unavailable-notice");
			await expect
				.element(notice)
				.toHaveTextContent(
					"Source content is temporarily unavailable. Please try again shortly.",
				);
			await expect.element(notice).not.toHaveTextContent("circuit breaker");
		});

		it("offers both remedies — a retry control and the original site", async () => {
			// NN/g heuristic 9: naming the problem without offering a way out is
			// the defect, not the message.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await page.getByRole("button", { name: /^article$/i }).click();

			await expect
				.element(page.getByTestId("retry-content"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("read-original-link"))
				.toHaveAttribute("href", mockFeed.link);
			// ...and the RSS body stays on screen underneath, so the reader is
			// never looking at a bare error.
			await expect
				.element(page.getByTestId("article-fallback-summary"))
				.toBeInTheDocument();
		});
	});

	describe("the one automatic foreground retry (ADR-000959 §4 / ADR-000963 §2)", () => {
		it("retries an unstamped ResourceExhausted once, showing the retrying state", async () => {
			// Unstamped means Alt's own host-slot gate rejected it: no packet
			// reached the publisher and Retry-After says when the slot frees.
			vi.mocked(getFeedContentOnTheFlyClient)
				.mockRejectedValueOnce(
					connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
				)
				.mockResolvedValue({
					content: "<p>Arrived on the second attempt.</p>",
					article_id: "article-123",
				} as never);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await page.getByRole("button", { name: /^article$/i }).click();

			await expect
				.element(page.getByTestId("article-content-pending"))
				.toHaveTextContent("Retrying");

			await expect
				.element(page.getByText("Arrived on the second attempt."))
				.toBeInTheDocument();
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);
		});

		// These count the requests the card sends ON ITS OWN, so none of them
		// touches the card: a reader's tap is a separate, deliberate request
		// that ADR-000963 §6 rations rather than refuses, and counting it here
		// would measure the wrong thing.
		it("stops after exactly one automatic retry, then states the failure", async () => {
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await new Promise((r) => setTimeout(r, 1200));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);

			await page.getByRole("button", { name: /^article$/i }).click();
			await expect
				.element(page.getByTestId("article-content-failed"))
				.toBeInTheDocument();
		});

		it("does NOT retry a ResourceExhausted stamped as the publisher's own 429", async () => {
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.ResourceExhausted, {
					"Retry-After": "0",
					"X-Alt-Failure-Scope": "host",
				}),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await new Promise((r) => setTimeout(r, 1200));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
		});

		it("does NOT retry Code.Unavailable — ADR-000959 rejected exactly that", async () => {
			// Two components re-sending into an open circuit breaker was the
			// incident. This test exists to keep it closed.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable, { "Retry-After": "0" }),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await new Promise((r) => setTimeout(r, 1200));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
		});

		it("still lets the reader ask again after an Unavailable failure", async () => {
			// The auto-retry ban is on the card re-sending by itself. A tap is
			// the reader deciding, which ADR-000963 §6 keeps reachable — the
			// "TRY AGAIN that does nothing" was half of that incident.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				connectError(Code.Unavailable),
			);

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					initialArticleContent: null,
					getCachedContent: () => null,
				},
			});

			await page.getByRole("button", { name: /^article$/i }).click();
			const retry = page.getByTestId("retry-content");
			await expect.element(retry).toBeInTheDocument();

			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "<p>Back on its feet.</p>",
				article_id: "article-123",
			} as never);
			await retry.click();

			await expect
				.element(page.getByText("Back on its feet."))
				.toBeInTheDocument();
		});
	});
});
