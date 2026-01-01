import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

// Mock the API client
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(),
}));

import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import { ArticlePrefetcher } from "./articlePrefetcher";

const mockGetFeedContentOnTheFlyClient = vi.mocked(getFeedContentOnTheFlyClient);

describe("ArticlePrefetcher", () => {
	let prefetcher: ArticlePrefetcher;

	beforeEach(() => {
		vi.clearAllMocks();
		vi.useFakeTimers();
		prefetcher = new ArticlePrefetcher();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	describe("getCachedArticleId", () => {
		it("should return null for non-cached URL", () => {
			const result = prefetcher.getCachedArticleId("https://example.com/article");
			expect(result).toBeNull();
		});

		it("should return cached article_id after prefetch", async () => {
			const feedUrl = "https://example.com/article";
			const expectedArticleId = "test-article-id-123";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: expectedArticleId,
			});

			const mockFeed = {
				id: "1",
				title: "Test Article",
				description: "Test description",
				link: feedUrl,
				published: "2024-01-01",
				normalizedUrl: feedUrl,
				publishedAtFormatted: "Jan 1, 2024",
				mergedTagsLabel: "",
				excerpt: "Test description",
			};

			// Trigger prefetch
			prefetcher.triggerPrefetch([mockFeed], -1, 1);

			// Advance timer past PREFETCH_DELAY (500ms)
			await vi.advanceTimersByTimeAsync(600);

			// Verify content was cached
			expect(prefetcher.getCachedContent(feedUrl)).toBe("<p>Test content</p>");

			// Verify article_id was cached
			expect(prefetcher.getCachedArticleId(feedUrl)).toBe(expectedArticleId);
		});

		it("should not cache article_id when API returns empty article_id", async () => {
			const feedUrl = "https://example.com/article";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "",
			});

			const mockFeed = {
				id: "1",
				title: "Test Article",
				description: "Test description",
				link: feedUrl,
				published: "2024-01-01",
				normalizedUrl: feedUrl,
				publishedAtFormatted: "Jan 1, 2024",
				mergedTagsLabel: "",
				excerpt: "Test description",
			};

			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			expect(prefetcher.getCachedContent(feedUrl)).toBe("<p>Test content</p>");
			expect(prefetcher.getCachedArticleId(feedUrl)).toBeNull();
		});
	});
});
