import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

// Mock the API client
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(),
}));

import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import { ArticlePrefetcher } from "./articlePrefetcher";

const mockGetFeedContentOnTheFlyClient = vi.mocked(
	getFeedContentOnTheFlyClient,
);

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
			const result = prefetcher.getCachedArticleId(
				"https://example.com/article",
			);
			expect(result).toBeNull();
		});

		it("should return cached article_id after prefetch", async () => {
			const feedUrl = "https://example.com/article";
			const expectedArticleId = "test-article-id-123";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: expectedArticleId,
				og_image_url: "",
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
				og_image_url: "",
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

	describe("getCachedOgImage", () => {
		it("should return null for non-cached URL", () => {
			expect(prefetcher.getCachedOgImage("https://example.com/article")).toBeNull();
		});

		it("should return cached og_image_url after prefetch", async () => {
			const feedUrl = "https://example.com/article";
			const expectedOgImage = "https://example.com/og.png";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "test-id",
				og_image_url: expectedOgImage,
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

			expect(prefetcher.getCachedOgImage(feedUrl)).toBe(expectedOgImage);
		});

		it("should cache null when og_image_url is empty", async () => {
			const feedUrl = "https://example.com/article";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "test-id",
				og_image_url: "",
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

			// Empty string should be stored as null
			expect(prefetcher.getCachedOgImage(feedUrl)).toBeNull();
		});
	});

	describe("onOgImageFetched callback", () => {
		it("should invoke callback when og_image_url is cached after prefetch", async () => {
			const feedUrl = "https://example.com/article";
			const callback = vi.fn();

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "test-id",
				og_image_url: "https://example.com/og.png",
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

			prefetcher.setOnOgImageFetched(callback);
			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			expect(callback).toHaveBeenCalledTimes(1);
		});

		it("should invoke callback even when og_image_url is empty", async () => {
			const feedUrl = "https://example.com/article";
			const callback = vi.fn();

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "test-id",
				og_image_url: "",
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

			prefetcher.setOnOgImageFetched(callback);
			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			expect(callback).toHaveBeenCalledTimes(1);
		});

		it("should not invoke callback when cleared with null", async () => {
			const feedUrl = "https://example.com/article";
			const callback = vi.fn();

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Test content</p>",
				article_id: "test-id",
				og_image_url: "https://example.com/og.png",
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

			prefetcher.setOnOgImageFetched(callback);
			prefetcher.setOnOgImageFetched(null);
			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			expect(callback).not.toHaveBeenCalled();
		});
	});

	describe("seedCache", () => {
		it("should store content, articleId, and ogImage retrievable via getters", () => {
			const feedUrl = "https://example.com/seeded";
			prefetcher.seedCache(feedUrl, "<p>Seeded</p>", "seed-id-1", "https://example.com/og.png");

			expect(prefetcher.getCachedContent(feedUrl)).toBe("<p>Seeded</p>");
			expect(prefetcher.getCachedArticleId(feedUrl)).toBe("seed-id-1");
			expect(prefetcher.getCachedOgImage(feedUrl)).toBe("https://example.com/og.png");
		});

		it("should invoke onOgImageFetched callback", () => {
			const callback = vi.fn();
			prefetcher.setOnOgImageFetched(callback);

			prefetcher.seedCache("https://example.com/seeded", "<p>Content</p>", "id-1", "https://example.com/og.png");

			expect(callback).toHaveBeenCalledTimes(1);
		});

		it("should store null ogImage when passed null", () => {
			prefetcher.seedCache("https://example.com/seeded", "<p>Content</p>", "id-1", null);

			expect(prefetcher.getCachedOgImage("https://example.com/seeded")).toBeNull();
		});
	});

	describe("cache eviction regression (OGP thumbnail fallback bug)", () => {
		it("should not evict active feed ogImage when prefetching 6 feeds ahead", async () => {
			// Regression test: MAX_CACHE_SIZE=5 < prefetchAhead+1=6 caused active feed eviction
			const activeFeedUrl = "https://example.com/active";

			// Seed the active feed into cache (simulates loadMore's first-feed fetch)
			prefetcher.seedCache(activeFeedUrl, "<p>Active content</p>", "active-id", "https://example.com/active-og.png");

			// Create 6 more feeds to prefetch ahead
			const feeds = [
				{ id: "0", title: "Active", description: "", link: activeFeedUrl, published: "", normalizedUrl: activeFeedUrl, publishedAtFormatted: "", mergedTagsLabel: "", excerpt: "" },
			];
			for (let i = 1; i <= 6; i++) {
				const url = `https://example.com/feed-${i}`;
				feeds.push({ id: `${i}`, title: `Feed ${i}`, description: "", link: url, published: "", normalizedUrl: url, publishedAtFormatted: "", mergedTagsLabel: "", excerpt: "" });

				mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
					content: `<p>Content ${i}</p>`,
					article_id: `id-${i}`,
					og_image_url: `https://example.com/og-${i}.png`,
				});
			}

			// Trigger prefetch for 6 feeds ahead from index 0
			prefetcher.triggerPrefetch(feeds, 0, 6);

			// Wait for all prefetches to complete (6 * 500ms delay + resolution time)
			await vi.advanceTimersByTimeAsync(4000);

			// The active feed's OGP image must NOT have been evicted
			expect(prefetcher.getCachedOgImage(activeFeedUrl)).toBe("https://example.com/active-og.png");
			expect(prefetcher.getCachedContent(activeFeedUrl)).toBe("<p>Active content</p>");
		});
	});

	describe("cache key consistency (regression test for stale content bug)", () => {
		it("should use normalizedUrl as cache key, not link", async () => {
			// This test ensures that cache is keyed by normalizedUrl
			// to prevent stale content when feeds have different tracking parameters
			const normalizedUrl = "https://example.com/article";
			const linkWithTracking =
				"https://example.com/article?utm_source=test&utm_campaign=123";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Correct content</p>",
				article_id: "article-123",
				og_image_url: "https://example.com/og.png",
			});

			const mockFeed = {
				id: "1",
				title: "Test Article",
				description: "Test description",
				link: linkWithTracking, // link has tracking params
				published: "2024-01-01",
				normalizedUrl: normalizedUrl, // normalizedUrl is clean
				publishedAtFormatted: "Jan 1, 2024",
				mergedTagsLabel: "",
				excerpt: "Test description",
			};

			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			// Cache should be accessible via normalizedUrl, NOT via link
			expect(prefetcher.getCachedContent(normalizedUrl)).toBe(
				"<p>Correct content</p>",
			);
			expect(prefetcher.getCachedArticleId(normalizedUrl)).toBe("article-123");

			// API should have been called with normalizedUrl
			expect(mockGetFeedContentOnTheFlyClient).toHaveBeenCalledWith(
				normalizedUrl,
			);
		});

		it("should separate cache entries for different normalizedUrls", async () => {
			// Ensure feeds with different normalizedUrls have separate cache entries
			const feedA = {
				id: "1",
				title: "Article A",
				description: "Description A",
				link: "https://example.com/article-a?utm=1",
				published: "2024-01-01",
				normalizedUrl: "https://example.com/article-a",
				publishedAtFormatted: "Jan 1, 2024",
				mergedTagsLabel: "",
				excerpt: "Description A",
			};

			const feedB = {
				id: "2",
				title: "Article B",
				description: "Description B",
				link: "https://example.com/article-b?utm=2",
				published: "2024-01-02",
				normalizedUrl: "https://example.com/article-b",
				publishedAtFormatted: "Jan 2, 2024",
				mergedTagsLabel: "",
				excerpt: "Description B",
			};

			mockGetFeedContentOnTheFlyClient
				.mockResolvedValueOnce({
					content: "<p>Content A</p>",
					article_id: "id-a",
					og_image_url: "https://example.com/a.png",
				})
				.mockResolvedValueOnce({
					content: "<p>Content B</p>",
					article_id: "id-b",
					og_image_url: "https://example.com/b.png",
				});

			// Prefetch both feeds
			prefetcher.triggerPrefetch([feedA, feedB], -1, 2);
			await vi.advanceTimersByTimeAsync(1100); // Wait for both prefetches

			// Each feed should have its own cached content
			expect(prefetcher.getCachedContent(feedA.normalizedUrl)).toBe(
				"<p>Content A</p>",
			);
			expect(prefetcher.getCachedContent(feedB.normalizedUrl)).toBe(
				"<p>Content B</p>",
			);
			expect(prefetcher.getCachedArticleId(feedA.normalizedUrl)).toBe("id-a");
			expect(prefetcher.getCachedArticleId(feedB.normalizedUrl)).toBe("id-b");
		});

		it("should not return cached content when accessed with link instead of normalizedUrl", async () => {
			// This test verifies the fix: content should NOT be accessible via link
			// when it was cached with normalizedUrl
			const normalizedUrl = "https://example.com/article";
			const linkWithTracking = "https://example.com/article?utm_source=test";

			mockGetFeedContentOnTheFlyClient.mockResolvedValueOnce({
				content: "<p>Cached content</p>",
				article_id: "article-123",
				og_image_url: "https://example.com/og.png",
			});

			const mockFeed = {
				id: "1",
				title: "Test Article",
				description: "Test description",
				link: linkWithTracking,
				published: "2024-01-01",
				normalizedUrl: normalizedUrl,
				publishedAtFormatted: "Jan 1, 2024",
				mergedTagsLabel: "",
				excerpt: "Test description",
			};

			prefetcher.triggerPrefetch([mockFeed], -1, 1);
			await vi.advanceTimersByTimeAsync(600);

			// Content is cached with normalizedUrl
			expect(prefetcher.getCachedContent(normalizedUrl)).toBe(
				"<p>Cached content</p>",
			);

			// Content should NOT be accessible via link (different key)
			expect(prefetcher.getCachedContent(linkWithTracking)).toBeNull();
		});
	});
});
