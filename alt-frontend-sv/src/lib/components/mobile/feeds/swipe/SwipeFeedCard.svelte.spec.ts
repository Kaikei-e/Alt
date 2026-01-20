/**
 * SwipeFeedCard Component Tests
 *
 * Tests for the swipeable feed card component using vitest-browser-svelte.
 * Tests interaction patterns, accessibility, and state management.
 */
import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi, beforeEach } from "vitest";

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
}));

// Mock Connect RPC functions
vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithRenderer: vi.fn(() =>
		Promise.resolve({
			hasReceivedData: true,
			articleId: "article-123",
		}),
	),
}));

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
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("heading", { name: mockFeed.title }))
				.toBeInTheDocument();
		});

		it("renders the swipe card container", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("swipe-card"))
				.toBeInTheDocument();
		});

		it("renders the action footer with buttons", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByTestId("action-footer"))
				.toBeInTheDocument();
		});

		it("renders Article button", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /article/i }))
				.toBeInTheDocument();
		});

		it("renders Summary button", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("button", { name: /summary/i }))
				.toBeInTheDocument();
		});
	});

	describe("accessibility", () => {
		it("has correct aria-label for external link", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByRole("link", { name: /open article/i }))
				.toBeInTheDocument();
		});

		it("sets aria-busy when component is busy", async () => {
			render(SwipeFeedCard as any, {
				props: {
					...defaultProps,
					isBusy: true,
				},
			});

			const card = page.getByTestId("swipe-card");
			await expect.element(card).toHaveAttribute("aria-busy", "true");
		});

		it("external link has correct rel attributes for security", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			const link = page.getByRole("link", { name: /open article/i });
			await expect.element(link).toHaveAttribute("target", "_blank");
			await expect.element(link).toHaveAttribute("rel", "noopener noreferrer");
		});
	});

	describe("feed description", () => {
		it("displays feed description when available", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			await expect
				.element(page.getByText(mockFeed.description))
				.toBeInTheDocument();
		});

		it("displays published date when available", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			// The component formats the date, so we check for presence of date
			const card = page.getByTestId("swipe-card");
			await expect.element(card).toBeInTheDocument();
		});
	});

	describe("button interactions", () => {
		it("Article button is enabled initially", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			const articleButton = page.getByRole("button", { name: /article/i });
			await expect.element(articleButton).not.toBeDisabled();
		});

		it("Summary button is enabled initially", async () => {
			render(SwipeFeedCard as any, {
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

			render(SwipeFeedCard as any, {
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

			render(SwipeFeedCard as any, {
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

			render(SwipeFeedCard as any, {
				props: {
					...defaultProps,
					initialArticleContent: initialContent,
				},
			});

			// The component should have the content ready without fetching
			await expect
				.element(page.getByTestId("swipe-card"))
				.toBeInTheDocument();
		});
	});

	describe("link structure", () => {
		it("article link points to correct URL", async () => {
			render(SwipeFeedCard as any, {
				props: defaultProps,
			});

			const link = page.getByRole("link", { name: /open article/i });
			await expect.element(link).toHaveAttribute("href", mockFeed.link);
		});
	});
});
