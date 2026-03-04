/**
 * VisualFeedCard Component Tests
 *
 * Tests for the desktop visual preview card with OG image thumbnails.
 * Uses vitest-browser-svelte for component testing.
 */
import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi, beforeEach } from "vitest";

import type { RenderFeed } from "$lib/schema/feed";
import VisualFeedCard from "./VisualFeedCard.svelte";

function createMockFeed(overrides: Partial<RenderFeed> = {}): RenderFeed {
	return {
		id: "test-id",
		title: "Test Article Title",
		description: "A test article description",
		link: "https://example.com/article",
		published: "2026-03-01T00:00:00Z",
		created_at: "2026-03-01T00:00:00Z",
		author: "Test Author",
		articleId: "article-123",
		isRead: false,
		publishedAtFormatted: "Mar 1, 2026",
		mergedTagsLabel: "Technology / AI",
		normalizedUrl: "https://example.com/article",
		excerpt: "This is a short excerpt from the article.",
		ogImageProxyUrl: "https://proxy.example.com/image.jpg",
		...overrides,
	};
}

describe("VisualFeedCard", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("rendering", () => {
		it("renders title, excerpt, author, and date", async () => {
			const feed = createMockFeed();
			render(VisualFeedCard as any, { props: { feed, onSelect: vi.fn() } });

			await expect
				.element(page.getByText("Test Article Title"))
				.toBeInTheDocument();
			await expect
				.element(page.getByText("This is a short excerpt from the article."))
				.toBeInTheDocument();
			await expect.element(page.getByText("Test Author")).toBeInTheDocument();
			await expect.element(page.getByText("Mar 1, 2026")).toBeInTheDocument();
		});

		it("renders tags (max 2)", async () => {
			const feed = createMockFeed({ mergedTagsLabel: "Tech / AI / Science" });
			render(VisualFeedCard as any, { props: { feed, onSelect: vi.fn() } });

			await expect.element(page.getByText("Tech")).toBeInTheDocument();
			await expect.element(page.getByText("AI")).toBeInTheDocument();
			// Third tag should not be rendered
			await expect.element(page.getByText("Science")).not.toBeInTheDocument();
		});

		it("does not render tags section when no tags", async () => {
			const feed = createMockFeed({ mergedTagsLabel: "" });
			render(VisualFeedCard as any, { props: { feed, onSelect: vi.fn() } });

			await expect
				.element(page.getByTestId("tags-container"))
				.not.toBeInTheDocument();
		});
	});

	describe("thumbnail", () => {
		it("renders image when ogImageProxyUrl is provided", async () => {
			const feed = createMockFeed();
			render(VisualFeedCard as any, { props: { feed, onSelect: vi.fn() } });

			const img = page.getByTestId("card-image");
			await expect.element(img).toBeInTheDocument();
			await expect
				.element(img)
				.toHaveAttribute("src", "https://proxy.example.com/image.jpg");
		});

		it("shows fallback when no image URL provided", async () => {
			const feed = createMockFeed({ ogImageProxyUrl: undefined });
			render(VisualFeedCard as any, { props: { feed, onSelect: vi.fn() } });

			await expect
				.element(page.getByTestId("card-image"))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByTestId("image-fallback"))
				.toBeInTheDocument();
		});
	});

	describe("interactions", () => {
		it("calls onSelect when clicked", async () => {
			const onSelect = vi.fn();
			const feed = createMockFeed();
			render(VisualFeedCard as any, { props: { feed, onSelect } });

			await page.getByRole("button").click();
			expect(onSelect).toHaveBeenCalledWith(feed);
		});
	});

	describe("read state styling", () => {
		it("shows read badge when isRead is true", async () => {
			const feed = createMockFeed();
			render(VisualFeedCard as any, {
				props: { feed, onSelect: vi.fn(), isRead: true },
			});

			await expect.element(page.getByText("Read")).toBeInTheDocument();
		});

		it("does not show read badge when isRead is false", async () => {
			const feed = createMockFeed();
			render(VisualFeedCard as any, {
				props: { feed, onSelect: vi.fn(), isRead: false },
			});

			await expect.element(page.getByText("Read")).not.toBeInTheDocument();
		});
	});
});
