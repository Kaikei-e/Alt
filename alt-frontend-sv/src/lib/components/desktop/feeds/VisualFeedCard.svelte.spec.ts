/**
 * VisualFeedCard Component Tests
 *
 * The card used by the visual feed grid (`/feeds/visual-preview`). Beyond the
 * usual title/excerpt/click contract it owns a four-state image pipeline —
 * idle -> loading -> loaded | absent — driven by an IntersectionObserver so
 * that only on-screen cards spend the proxy's per-host rate-limit budget, plus
 * the AbortController / object-URL cleanup that pipeline requires.
 *
 * The distinction between "still loading" and "absent" is the interesting part:
 * a transient 429 is retried *inside* the loader and must keep the shimmer, and
 * must not collapse the card to the fallback gradient the way the old sticky
 * `onerror` handler did. That is asserted explicitly below.
 */
import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import type { RenderFeed } from "$lib/schema/feed";
import { renderFeedFixture } from "../../../../../tests/fixtures/feeds";
import VisualFeedCard from "./VisualFeedCard.svelte";

const { loadProxyImageDefault } = vi.hoisted(() => ({
	loadProxyImageDefault: vi.fn(),
}));

vi.mock("$lib/utils/loadProxyImage", () => ({ loadProxyImageDefault }));

const PROXY_URL = "/api/og-image?u=https%3A%2F%2Falt.ai%2Fog.png";

const feedWithImage: RenderFeed = {
	...renderFeedFixture,
	ogImageProxyUrl: PROXY_URL,
};

/**
 * A real blob URL backed by a 1x1 transparent GIF, so the rendered <img>
 * resolves for real and `URL.revokeObjectURL` is handed a URL it actually owns.
 */
function createBlobUrl(): string {
	const gif = "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";
	const bytes = Uint8Array.from(atob(gif), (char) => char.charCodeAt(0));
	return URL.createObjectURL(new Blob([bytes], { type: "image/gif" }));
}

/** Let mount effects, the IntersectionObserver callback and microtasks settle. */
function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 50));
}

describe("VisualFeedCard", () => {
	beforeEach(() => {
		loadProxyImageDefault.mockReset();
		// Default: no image resolves, so tests that do not care about the image
		// pipeline land on the fallback instead of hanging in "loading".
		loadProxyImageDefault.mockResolvedValue({ status: "absent" });
	});

	describe("content", () => {
		it("renders the feed title", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByText("Daily AI Recap"))
				.toBeInTheDocument();
		});

		it("renders the excerpt", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByText(/most important AI breakthroughs/))
				.toBeInTheDocument();
		});

		it("renders the author and the formatted publish date", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect.element(page.getByText("Alt AI Team")).toBeInTheDocument();
			await expect.element(page.getByText("Dec 22, 2025")).toBeInTheDocument();
		});

		it("omits the excerpt paragraph when the feed has none", async () => {
			render(VisualFeedCard, {
				props: {
					feed: { ...renderFeedFixture, excerpt: "" },
					onSelect: vi.fn(),
				},
			});

			await expect
				.element(page.getByText("Daily AI Recap"))
				.toBeInTheDocument();
			await expect
				.element(page.getByText(/most important AI breakthroughs/))
				.not.toBeInTheDocument();
		});
	});

	describe("tags", () => {
		it("renders at most the first two tags of mergedTagsLabel", async () => {
			render(VisualFeedCard, {
				props: {
					feed: {
						...renderFeedFixture,
						mergedTagsLabel: "AI / Research / Ethics",
					},
					onSelect: vi.fn(),
				},
			});

			// Scoped to the tag container: "AI" and "ethics" also occur in the
			// title / excerpt / author, so an unscoped text query would pass on
			// the wrong element and never notice the slice(0, 2).
			const tags = page.getByTestId("tags-container");
			await expect
				.element(tags.getByText("AI", { exact: true }))
				.toBeInTheDocument();
			await expect
				.element(tags.getByText("Research", { exact: true }))
				.toBeInTheDocument();
			await expect
				.element(tags.getByText("Ethics", { exact: true }))
				.not.toBeInTheDocument();
		});

		it("omits the tag container when the feed has no tags", async () => {
			render(VisualFeedCard, {
				props: {
					feed: { ...renderFeedFixture, mergedTagsLabel: "" },
					onSelect: vi.fn(),
				},
			});

			await expect
				.element(page.getByText("Daily AI Recap"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("tags-container"))
				.not.toBeInTheDocument();
		});
	});

	describe("read state", () => {
		it("shows the read indicator when the feed is read", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, isRead: true, onSelect: vi.fn() },
			});

			await expect.element(page.getByText("Read")).toBeInTheDocument();
		});

		it("hides the read indicator when the feed is unread", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, isRead: false, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByText("Daily AI Recap"))
				.toBeInTheDocument();
			await expect.element(page.getByText("Read")).not.toBeInTheDocument();
		});

		it("defaults to unread when isRead is omitted", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByText("Daily AI Recap"))
				.toBeInTheDocument();
			await expect.element(page.getByText("Read")).not.toBeInTheDocument();
		});
	});

	describe("selection", () => {
		it("exposes the card as a button labelled with the feed title", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByRole("button", { name: "Open Daily AI Recap" }))
				.toBeInTheDocument();
		});

		it("calls onSelect with the feed when clicked", async () => {
			const onSelect = vi.fn();
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect },
			});

			await page.getByRole("button", { name: "Open Daily AI Recap" }).click();

			expect(onSelect).toHaveBeenCalledTimes(1);
			expect(onSelect).toHaveBeenCalledWith(renderFeedFixture);
		});
	});

	describe("image pipeline", () => {
		it("shows the fallback gradient and never loads when the feed has no image", async () => {
			render(VisualFeedCard, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("image-fallback"))
				.toBeInTheDocument();

			// An imageless card must not spend proxy rate-limit budget.
			await settle();
			expect(loadProxyImageDefault).not.toHaveBeenCalled();
		});

		it("renders the image once the proxy load resolves", async () => {
			const objectUrl = createBlobUrl();
			loadProxyImageDefault.mockResolvedValue({ status: "loaded", objectUrl });

			render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			const image = page.getByTestId("card-image");
			await expect.element(image).toBeInTheDocument();
			await expect.element(image).toHaveAttribute("src", objectUrl);
			await expect
				.element(page.getByTestId("image-fallback"))
				.not.toBeInTheDocument();

			expect(loadProxyImageDefault).toHaveBeenCalledWith(
				PROXY_URL,
				expect.any(AbortSignal),
			);
		});

		it("falls back to the gradient when the proxy load resolves absent", async () => {
			loadProxyImageDefault.mockResolvedValue({ status: "absent" });

			render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			// This must observe the *transition* into "absent", not merely a frame
			// in which the fallback happens to be on screen: the pre-load "idle"
			// state also renders the fallback for an imageless card, so a bare
			// retrying `toBeInTheDocument` is satisfied by the first paint and
			// would still pass if "absent" never reached the fallback at all.
			// Waiting for the loader to have run leaves "idle" behind, and
			// requiring the shimmer to be gone leaves "loading" behind.
			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			await expect
				.element(page.getByTestId("image-loading"))
				.not.toBeInTheDocument();

			await expect
				.element(page.getByTestId("image-fallback"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("card-image"))
				.not.toBeInTheDocument();
		});

		it("keeps the shimmer while a load is in flight instead of collapsing to the fallback", async () => {
			// Never resolves: models a 429 being retried with backoff inside the
			// loader. The old sticky onerror handler dropped straight to the
			// fallback here, which is the regression this pins down.
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			// The call itself proves the card was detected in view, so the shimmer
			// below is the "loading" state and not merely the untouched "idle" one.
			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);

			await expect
				.element(page.getByTestId("image-loading"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("image-fallback"))
				.not.toBeInTheDocument();
		});

		it("restarts the load when the feed's proxy URL changes", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { rerender } = render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);

			const nextUrl = "/api/og-image?u=https%3A%2F%2Falt.ai%2Fother.png";
			await rerender({
				feed: { ...feedWithImage, id: "feed-2", ogImageProxyUrl: nextUrl },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(2),
			);
			expect(loadProxyImageDefault).toHaveBeenLastCalledWith(
				nextUrl,
				expect.any(AbortSignal),
			);
		});

		it("aborts the superseded load when the feed's proxy URL changes", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { rerender } = render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			const staleSignal = loadProxyImageDefault.mock
				.calls[0]?.[1] as AbortSignal;
			expect(staleSignal.aborted).toBe(false);

			await rerender({
				feed: {
					...feedWithImage,
					id: "feed-2",
					ogImageProxyUrl: "/api/og-image?u=https%3A%2F%2Falt.ai%2Fother.png",
				},
			});

			// Restarting the load is not enough on its own — the load effect would
			// restart anyway on the URL change. The previous request must also be
			// cancelled, or a slow response for the old feed can resolve after the
			// new one and paint the previous feed's image into the recycled card.
			await vi.waitFor(() => expect(staleSignal.aborted).toBe(true));
		});
	});

	describe("cleanup", () => {
		it("aborts an in-flight image load when destroyed", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { unmount } = render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			const signal = loadProxyImageDefault.mock.calls[0]?.[1] as AbortSignal;
			expect(signal.aborted).toBe(false);

			unmount();

			expect(signal.aborted).toBe(true);
		});

		it("revokes the object URL when destroyed", async () => {
			const objectUrl = createBlobUrl();
			loadProxyImageDefault.mockResolvedValue({ status: "loaded", objectUrl });
			const revokeSpy = vi.spyOn(URL, "revokeObjectURL");

			const { unmount } = render(VisualFeedCard, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await expect.element(page.getByTestId("card-image")).toBeInTheDocument();

			unmount();

			expect(revokeSpy).toHaveBeenCalledWith(objectUrl);
			revokeSpy.mockRestore();
		});
	});
});
