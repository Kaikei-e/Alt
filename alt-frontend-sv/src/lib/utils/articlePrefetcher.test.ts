import { describe, it, expect, vi, beforeEach } from "vitest";

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
