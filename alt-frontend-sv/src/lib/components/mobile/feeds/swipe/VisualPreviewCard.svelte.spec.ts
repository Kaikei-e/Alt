/**
 * VisualPreviewCard Component Tests
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { RenderFeed } from "#lib/schema/feed.js";
import VisualPreviewCard from "./VisualPreviewCard.svelte";

const { loadProxyImageDefault, resolveOgImage } = vi.hoisted(() => ({
	loadProxyImageDefault: vi.fn(),
	resolveOgImage: vi.fn(),
}));

vi.mock("#lib/utils/loadProxyImage.js", () => ({ loadProxyImageDefault }));
vi.mock("#lib/utils/ogImageResolver.js", () => ({
	ogImageResolver: () => ({ resolve: resolveOgImage }),
}));

const PROXY_URL =
	"/v1/images/proxy/testsig/aHR0cHM6Ly9jZG4uZXhhbXBsZS5jb20vaGVyby5qcGc";

function settle(): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, 50));
}

const mockFeed: RenderFeed = {
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

describe("VisualPreviewCard", () => {
	const defaultProps = {
		feed: mockFeed,
		statusMessage: null,
		onDismiss: vi.fn(),
		onOpenArticle: vi.fn(),
		thumbnailProxyUrl: PROXY_URL,
	};

	beforeEach(() => {
		vi.clearAllMocks();
		loadProxyImageDefault.mockReset();
		loadProxyImageDefault.mockResolvedValue({ status: "loaded" });
		resolveOgImage.mockReset();
		resolveOgImage.mockResolvedValue({ status: "absent" });
	});

	describe("rendering and removed in-card reader controls", () => {
		it("renders card title, description, and Article button without Keep stamp or Summary button", async () => {
			render(VisualPreviewCard, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("heading", { name: mockFeed.title }))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("visual-preview-card"))
				.toBeInTheDocument();
			await expect
				.element(page.getByTestId("action-footer"))
				.toBeInTheDocument();
			await expect
				.element(page.getByRole("button", { name: /^article$/i }))
				.toBeInTheDocument();
			await expect
				.element(page.getByText(mockFeed.description))
				.toBeInTheDocument();
			await expect
				.element(page.getByText("Swipe to mark as read"))
				.toBeInTheDocument();

			await expect
				.element(page.getByTestId("keep-stamp"))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole("button", { name: /summary/i }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByTestId("summary-speech-button"))
				.not.toBeInTheDocument();
		});
	});

	describe("opening the reader overlay", () => {
		it("calls onOpenArticle when Article button is clicked", async () => {
			const onOpenArticle = vi.fn();
			render(VisualPreviewCard, {
				props: { ...defaultProps, onOpenArticle },
			});

			await page.getByRole("button", { name: /^article$/i }).click();
			expect(onOpenArticle).toHaveBeenCalledTimes(1);
		});

		it("calls onOpenArticle when the card surface is tapped", async () => {
			const onOpenArticle = vi.fn();
			render(VisualPreviewCard, {
				props: { ...defaultProps, onOpenArticle },
			});

			await page.getByText(mockFeed.description).click();
			expect(onOpenArticle).toHaveBeenCalledTimes(1);
		});

		it("does not call onOpenArticle when the external title link is clicked", async () => {
			const onOpenArticle = vi.fn();
			render(VisualPreviewCard, {
				props: { ...defaultProps, onOpenArticle },
			});

			const link = page.getByRole("link", { name: /open article/i });
			(link.element() as HTMLAnchorElement).addEventListener("click", (e) => {
				e.preventDefault();
			});
			await link.click();

			expect(onOpenArticle).not.toHaveBeenCalled();
		});
	});

	describe("thumbnail pipeline", () => {
		it("renders the thumbnail once the proxy load resolves", async () => {
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
});
