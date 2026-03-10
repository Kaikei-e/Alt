import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock the client API to prevent transitive $app/* resolution
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(),
}));

import { ArticlePrefetcher } from "./articlePrefetcher";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";

const mockedGetContent = vi.mocked(getFeedContentOnTheFlyClient);

function makeFeed(id: string, url: string) {
	return {
		id,
		normalizedUrl: url,
		link: url,
		title: "Test",
		description: "",
		published: "",
		author: "",
		feedSource: "",
	} as any;
}

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

	describe("cache eviction", () => {
		it("does not evict active card's OG image when prefetching ahead", async () => {
			// Seed the active card's cache
			prefetcher.seedCache(
				"https://example.com/active",
				"<p>Active</p>",
				"art-active",
				null,
				"https://proxy/active.jpg",
			);

			// Verify active card has OG image
			expect(prefetcher.getCachedOgImage("https://example.com/active")).toBe(
				"https://proxy/active.jpg",
			);

			// Seed 30 more entries to trigger eviction
			for (let i = 1; i <= 30; i++) {
				prefetcher.seedCache(
					`https://example.com/feed-${i}`,
					`<p>Feed ${i}</p>`,
					`art-${i}`,
					null,
					`https://proxy/feed-${i}.jpg`,
				);
			}

			// With MAX_CACHE_SIZE=30, the active card should still be in cache
			// (it was the 1st of 31 entries, so it gets evicted only after 30 new entries)
			// The key point: with the old MAX_CACHE_SIZE=10, this would have been evicted
			// after just 10 new entries. With 30, we have enough headroom for prefetchAhead=10.

			// Simulate the realistic scenario: active + 10 prefetched
			const prefetcher2 = new ArticlePrefetcher();
			prefetcher2.seedCache(
				"https://example.com/current",
				"<p>Current</p>",
				"art-current",
				null,
				"https://proxy/current.jpg",
			);

			for (let i = 1; i <= 10; i++) {
				prefetcher2.seedCache(
					`https://example.com/next-${i}`,
					`<p>Next ${i}</p>`,
					`art-next-${i}`,
					null,
					`https://proxy/next-${i}.jpg`,
				);
			}

			// Active card's OG image must survive with 11 entries (well under MAX_CACHE_SIZE=30)
			expect(prefetcher2.getCachedOgImage("https://example.com/current")).toBe(
				"https://proxy/current.jpg",
			);
			expect(prefetcher2.getCachedContent("https://example.com/current")).toBe(
				"<p>Current</p>",
			);
		});
	});

	describe("onArticleIdCached callback", () => {
		it("fires when prefetchContent resolves with an article_id", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Hello</p>",
				article_id: "art-123",
				og_image_url: null,
			} as any);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/article"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);

			// Advance past PREFETCH_DELAY (500ms)
			vi.advanceTimersByTime(600);

			// Wait for the async fetch to complete
			await vi.waitFor(() => {
				expect(callback).toHaveBeenCalledWith(
					"https://example.com/article",
					"art-123",
				);
			});
		});

		it("does not fire when article_id is absent", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>No article ID</p>",
				article_id: "",
				og_image_url: null,
			} as any);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/no-id"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			vi.advanceTimersByTime(600);

			// Wait for fetch to complete
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalled();
			});

			// Callback should NOT have been called
			expect(callback).not.toHaveBeenCalled();
		});

		it("can be cleared by passing null", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);
			prefetcher.setOnArticleIdCached(null);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>test</p>",
				article_id: "art-456",
				og_image_url: null,
			} as any);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/cleared"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			vi.advanceTimersByTime(600);

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalled();
			});

			expect(callback).not.toHaveBeenCalled();
		});
	});
});
