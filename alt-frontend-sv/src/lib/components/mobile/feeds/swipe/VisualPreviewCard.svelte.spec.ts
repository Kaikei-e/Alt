/**
 * VisualPreviewCard Component Tests
 *
 * The card the mobile swipe surface (`/feeds/swipe/visual-preview`) shows one
 * at a time. Besides the article-body panel it owns the same four-state image
 * pipeline the desktop grid and the mobile gallery use — idle -> loading ->
 * loaded | absent, via `createProxyImage` — and the two properties that
 * pipeline exists for are pinned below:
 *
 *  - **A transient failure is not "absent".** A 429 from the shared image
 *    proxy is retried *inside* `loadProxyImage`, and the card must hold its
 *    shimmer while that happens. The card used to render a bare `<img>` with
 *    an `onerror` that latched `imgError = true` for the life of the card, so
 *    one rate-limited response pinned it to "No preview" for the rest of the
 *    session. That is the regression this suite guards.
 *  - **Only a proxy URL is ever rendered.** What reaches `<img src>` is the
 *    URL `loadProxyImage` actually probed, which is the HMAC-signed
 *    `/v1/images/proxy/...` path. A raw publisher URL used to arrive here from
 *    `FetchArticleContent.og_image_url` and go straight into the DOM as an
 *    unproxied cross-origin request.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { page } from "@vitest/browser/context";
import { flushSync, tick } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { RenderFeed } from "$lib/schema/feed";
import VisualPreviewCard from "./VisualPreviewCard.svelte";

const { loadProxyImageDefault, resolveOgImage } = vi.hoisted(() => ({
	loadProxyImageDefault: vi.fn(),
	resolveOgImage: vi.fn(),
}));

vi.mock("$lib/utils/loadProxyImage", () => ({ loadProxyImageDefault }));

// Stubbed rather than left to the real shared resolver: without this the card
// fires a Connect-RPC at the dev server and the outcome of every image test
// depends on what that happens to answer.
vi.mock("$lib/utils/ogImageResolver", () => ({
	ogImageResolver: () => ({ resolve: resolveOgImage }),
}));

/** What the image proxy hands out: an HMAC signature over an encoded URL. */
const PROXY_URL =
	"/v1/images/proxy/testsig/aHR0cHM6Ly9jZG4uZXhhbXBsZS5jb20vaGVyby5qcGc";

/** Let mount effects, the IntersectionObserver callback and microtasks settle. */
function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 50));
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

const mockFeed: RenderFeed = {
	// `id` is articles.id (or a per-response UUID) and is a render key only;
	// `feedId` is feeds.id, the one ResolveOgImages matches. They are set to
	// different strings on purpose — a fixture where they agree would keep
	// passing if the card regressed to sending `id`, which is the bug that
	// shipped once already.
	id: "feed-test-1",
	feedId: "feeds-row-uuid-1",
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
		thumbnailProxyUrl: PROXY_URL,
	};

	beforeEach(() => {
		vi.clearAllMocks();
		// `clearAllMocks` clears calls but keeps implementations, so these are
		// re-stated rather than relying on what a previous test left behind.
		loadProxyImageDefault.mockReset();
		loadProxyImageDefault.mockResolvedValue({ status: "loaded" });
		resolveOgImage.mockReset();
		resolveOgImage.mockResolvedValue({ status: "absent" });
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

	describe("thumbnail pipeline", () => {
		it("renders the thumbnail once the proxy load resolves", async () => {
			// Replaces "renders thumbnail image when URL is provided": having a
			// URL is no longer what puts an <img> on screen — a load that
			// answered 200 is.
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("thumbnail-image"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("thumbnail-fallback"))
				.not.toBeInTheDocument();
		});

		it("renders the fallback when there is no image to be had", async () => {
			// No URL and nothing to resolve one from: the card settles rather
			// than shimmering for the rest of the session.
			resolveOgImage.mockResolvedValue({ status: "absent" });

			render(VisualPreviewCard, {
				props: {
					...defaultProps,
					thumbnailProxyUrl: null,
				},
			});

			await expect
				.element(page.getByTestId("thumbnail-fallback"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("thumbnail-image"))
				.not.toBeInTheDocument();
		});

		it("renders exactly the proxy URL the load probed, never a publisher URL", async () => {
			// Replaces the old assertion that src === "https://cdn.example.com/
			// hero.jpg". A raw cross-origin URL in <img src> is the defect, not
			// the contract: what is rendered is the signed /v1/images/proxy path
			// that `loadProxyImage` verified, and it is the same string, so a
			// preload hint can match it.
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("src", PROXY_URL);

			const src = (img.element() as HTMLImageElement).getAttribute("src");
			expect(src).toMatch(/^\/v1\/images\/proxy\//);
			expect(loadProxyImageDefault).toHaveBeenCalledWith(
				src,
				expect.any(AbortSignal),
			);
		});

		it("keeps the shimmer while a transient failure is retried inside the loader", async () => {
			// Never resolves: models a 429 being retried with backoff inside
			// `loadProxyImage`. The card used to latch `imgError` on the first
			// `onerror` and show "No preview" for the rest of the session — an
			// <img> reports a retryable 429 and a permanent 403 as the same
			// silent event, so the card could not tell them apart at all.
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);

			await expect
				.element(page.getByTestId("thumbnail-shimmer"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("thumbnail-fallback"))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByTestId("thumbnail-image"))
				.not.toBeInTheDocument();
		});

		it("settles on the fallback once the loader gives up for good", async () => {
			loadProxyImageDefault.mockResolvedValue({ status: "absent" });

			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);

			await expect
				.element(page.getByTestId("thumbnail-fallback"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("thumbnail-shimmer"))
				.not.toBeInTheDocument();
		});

		it("carries the LCP hints on the first card only", async () => {
			// Replaces "thumbnail image has lazy loading". Both halves of the
			// hint matter now that `src` is a real URL the browser can act on:
			// the first card is the LCP element and must not be lazy.
			render(VisualPreviewCard, {
				props: { ...defaultProps, isLcp: true },
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("loading", "eager");
			await expect.element(img).toHaveAttribute("fetchpriority", "high");
		});

		it("leaves later cards lazy and unprioritised", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("loading", "lazy");
			await expect.element(img).not.toHaveAttribute("fetchpriority");
		});
	});

	describe("on-demand og:image resolution", () => {
		it("asks on feeds.id, never on the render key", async () => {
			// `id` is articles.id or a per-response UUID; ResolveOgImages matches
			// feeds.id. Sending `id` matched nothing and read back as "no feed
			// has an image" — a shipped bug.
			resolveOgImage.mockResolvedValue({ status: "resolved", url: PROXY_URL });

			render(VisualPreviewCard, {
				props: { ...defaultProps, thumbnailProxyUrl: null },
			});

			await vi.waitFor(() => expect(resolveOgImage).toHaveBeenCalled());
			expect(resolveOgImage).toHaveBeenCalledWith("feeds-row-uuid-1");
			expect(resolveOgImage).not.toHaveBeenCalledWith(mockFeed.id);

			await expect
				.element(page.getByTestId("thumbnail-image"))
				.toHaveAttribute("src", PROXY_URL);
		});

		it("does not resolve when the card already has a proxy URL", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await settle();
			expect(resolveOgImage).not.toHaveBeenCalled();
		});
	});

	describe("cleanup", () => {
		it("aborts an in-flight image load when the card is destroyed", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { unmount } = render(VisualPreviewCard, {
				props: defaultProps,
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			const signal = loadProxyImageDefault.mock.calls[0]?.[1] as AbortSignal;
			expect(signal.aborted).toBe(false);

			unmount();

			expect(signal.aborted).toBe(true);
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
