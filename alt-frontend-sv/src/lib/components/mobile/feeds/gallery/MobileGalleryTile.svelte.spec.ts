/**
 * MobileGalleryTile Component Tests
 *
 * The tile used by the mobile visual-preview gallery (`/feeds/visual-preview`
 * at phone width). It is deliberately NOT a shrunken VisualFeedCard: at half a
 * phone's width there is no room for an excerpt, an author line and a tag row
 * without pushing the image down to a size NN/g's mobile-list research says
 * stops being useful. The image leads; the title is the only prose.
 *
 * The four-state image pipeline (idle -> loading -> loaded | absent) is shared
 * with VisualFeedCard via `createProxyImage`, and the same distinction matters
 * here: a transient 429 is retried *inside* the loader and must keep the
 * shimmer rather than collapsing to the fallback.
 */
import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import type { RenderFeed } from "$lib/schema/feed";
import { renderFeedFixture } from "../../../../../../tests/fixtures/feeds";
import MobileGalleryTile from "./MobileGalleryTile.svelte";

const { loadProxyImageDefault } = vi.hoisted(() => ({
	loadProxyImageDefault: vi.fn(),
}));

vi.mock("$lib/utils/loadProxyImage", () => ({ loadProxyImageDefault }));

const PROXY_URL = "/api/og-image?u=https%3A%2F%2Falt.ai%2Fog.png";

const feedWithImage: RenderFeed = {
	...renderFeedFixture,
	ogImageProxyUrl: PROXY_URL,
};

/** Let mount effects, the IntersectionObserver callback and microtasks settle. */
function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 50));
}

describe("MobileGalleryTile", () => {
	beforeEach(() => {
		loadProxyImageDefault.mockReset();
		loadProxyImageDefault.mockResolvedValue({ status: "absent" });
	});

	describe("content", () => {
		it("renders the feed title", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("tile-title"))
				.toHaveTextContent("Daily AI Recap");
		});

		it("renders the formatted publish date", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect.element(page.getByText("Dec 22, 2025")).toBeInTheDocument();
		});

		it("omits the excerpt so the thumbnail keeps the vertical budget", async () => {
			// Half-width tiles: an excerpt here costs image height, which is the
			// one thing the gallery exists to show. This is a design constraint,
			// not an oversight, so it is pinned.
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect.element(page.getByTestId("tile-title")).toBeInTheDocument();
			await expect
				.element(page.getByText(/most important AI breakthroughs/))
				.not.toBeInTheDocument();
		});
	});

	describe("read state", () => {
		it("marks the tile as read", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, isRead: true, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("gallery-tile"))
				.toHaveAttribute("data-read", "true");
			await expect
				.element(page.getByTestId("tile-unread-dot"))
				.not.toBeInTheDocument();
		});

		it("shows the unread marker when the feed is unread", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, isRead: false, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("gallery-tile"))
				.toHaveAttribute("data-read", "false");
			await expect
				.element(page.getByTestId("tile-unread-dot"))
				.toBeInTheDocument();
		});

		it("defaults to unread when isRead is omitted", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("gallery-tile"))
				.toHaveAttribute("data-read", "false");
		});
	});

	describe("selection", () => {
		it("exposes the whole tile as a button labelled with the feed title", async () => {
			// The image carries no alt text — it is decorative next to the title —
			// so the tile's own label is the only accessible name a screen-reader
			// user gets for the target.
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByRole("button", { name: "Open Daily AI Recap" }))
				.toBeInTheDocument();
		});

		it("calls onSelect with the feed when tapped", async () => {
			const onSelect = vi.fn();
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect },
			});

			await page.getByRole("button", { name: "Open Daily AI Recap" }).click();

			expect(onSelect).toHaveBeenCalledTimes(1);
			expect(onSelect).toHaveBeenCalledWith(renderFeedFixture);
		});
	});

	describe("image pipeline", () => {
		it("shows the fallback and never loads when the feed has no image", async () => {
			render(MobileGalleryTile, {
				props: { feed: renderFeedFixture, onSelect: vi.fn() },
			});

			await expect
				.element(page.getByTestId("tile-image-fallback"))
				.toBeInTheDocument();

			await settle();
			expect(loadProxyImageDefault).not.toHaveBeenCalled();
		});

		it("renders the image once the proxy load resolves", async () => {
			loadProxyImageDefault.mockResolvedValue({ status: "loaded" });

			render(MobileGalleryTile, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			const image = page.getByTestId("tile-image");
			await expect.element(image).toBeInTheDocument();
			await expect.element(image).toHaveAttribute("src", PROXY_URL);
			await expect
				.element(page.getByTestId("tile-image-fallback"))
				.not.toBeInTheDocument();

			expect(loadProxyImageDefault).toHaveBeenCalledWith(
				PROXY_URL,
				expect.any(AbortSignal),
			);
		});

		it("falls back when the proxy load resolves absent", async () => {
			loadProxyImageDefault.mockResolvedValue({ status: "absent" });

			render(MobileGalleryTile, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			// Observe the transition into "absent": the pre-load "idle" state also
			// paints the fallback, so a bare retrying assertion would be satisfied
			// by the first frame and prove nothing.
			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			await expect
				.element(page.getByTestId("tile-image-loading"))
				.not.toBeInTheDocument();

			await expect
				.element(page.getByTestId("tile-image-fallback"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("tile-image"))
				.not.toBeInTheDocument();
		});

		it("keeps the shimmer while a load is in flight", async () => {
			// Never resolves: models a 429 being retried with backoff inside the
			// loader. Collapsing to the fallback here is the regression that
			// PM-era sticky onerror handling caused on the desktop card.
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			render(MobileGalleryTile, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);

			await expect
				.element(page.getByTestId("tile-image-loading"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("tile-image-fallback"))
				.not.toBeInTheDocument();
		});

		it("aborts the superseded load when the feed's proxy URL changes", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { rerender } = render(MobileGalleryTile, {
				props: { feed: feedWithImage, onSelect: vi.fn() },
			});

			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(1),
			);
			const staleSignal = loadProxyImageDefault.mock
				.calls[0]?.[1] as AbortSignal;

			const nextUrl = "/api/og-image?u=https%3A%2F%2Falt.ai%2Fother.png";
			await rerender({
				feed: { ...feedWithImage, id: "feed-2", ogImageProxyUrl: nextUrl },
			});

			// A slow response for the previous feed must not paint into the
			// recycled tile after the new one has been assigned.
			await vi.waitFor(() => expect(staleSignal.aborted).toBe(true));
			await vi.waitFor(() =>
				expect(loadProxyImageDefault).toHaveBeenCalledTimes(2),
			);
			expect(loadProxyImageDefault).toHaveBeenLastCalledWith(
				nextUrl,
				expect.any(AbortSignal),
			);
		});
	});

	describe("cleanup", () => {
		it("aborts an in-flight image load when destroyed", async () => {
			loadProxyImageDefault.mockReturnValue(new Promise<never>(() => {}));

			const { unmount } = render(MobileGalleryTile, {
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
	});
});
