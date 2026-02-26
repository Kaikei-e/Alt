/**
 * VisualPreviewCard Component Tests
 *
 * Tests for the visual preview swipe card with thumbnail images.
 * Uses vitest-browser-svelte for component testing.
 */
import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi, beforeEach } from "vitest";

import type { RenderFeed } from "$lib/schema/feed";
import VisualPreviewCard from "./VisualPreviewCard.svelte";

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
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("heading", { name: mockFeed.title }))
				.toBeInTheDocument();
		});

		it("renders the swipe card container", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("visual-preview-card"))
				.toBeInTheDocument();
		});

		it("renders the action footer with buttons", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("action-footer"))
				.toBeInTheDocument();
		});

		it("renders Article button", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("renders Summary button", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /summary/i }))
				.toBeInTheDocument();
		});
	});

	describe("thumbnail rendering", () => {
		it("renders thumbnail image when URL is provided", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("thumbnail-image"))
				.toBeInTheDocument();
		});

		it("renders fallback gradient when thumbnailUrl is null", async () => {
			render(VisualPreviewCard as any, {
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
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("src", defaultProps.thumbnailUrl);
		});

		it("thumbnail image has lazy loading", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			const img = page.getByTestId("thumbnail-image");
			await expect.element(img).toHaveAttribute("loading", "lazy");
		});
	});

	describe("accessibility", () => {
		it("has correct aria-label for external link", async () => {
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("link", { name: /open article/i }))
				.toBeInTheDocument();
		});

		it("sets aria-busy when component is busy", async () => {
			render(VisualPreviewCard as any, {
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
			render(VisualPreviewCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByText(mockFeed.description))
				.toBeInTheDocument();
		});

		it("displays 'Swipe to mark as read' text", async () => {
			render(VisualPreviewCard as any, {
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

			render(VisualPreviewCard as any, {
				props: {
					...defaultProps,
					getCachedContent,
				},
			});

			expect(getCachedContent).toHaveBeenCalledWith(mockFeed.normalizedUrl);
		});
	});
});
