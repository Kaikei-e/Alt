import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Code, ConnectError } from "@connectrpc/connect";

// Mock the client API to prevent transitive $app/* resolution
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(),
}));

import { ArticlePrefetcher } from "./articlePrefetcher";
import type { RenderFeed } from "$lib/schema/feed";
import type { FeedContentOnTheFlyResponse } from "$lib/api/client/articles";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";

const mockedGetContent = vi.mocked(getFeedContentOnTheFlyClient);

function makeFeed(id: string, url: string): RenderFeed {
	return {
		id,
		normalizedUrl: url,
		link: url,
		title: "Test",
		description: "",
		published: "",
		author: "",
		feedSource: "",
	} as unknown as RenderFeed;
}

describe("ArticlePrefetcher", () => {
	let prefetcher: ArticlePrefetcher;

	beforeEach(() => {
		// resetAllMocks (not clearAllMocks) so mockResolvedValueOnce queues
		// from a previous test do not leak into the next.
		vi.resetAllMocks();
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

	describe("per-host serialization", () => {
		it("serializes prefetches to the same host (no parallel fan-out)", async () => {
			let resolveFirst: (value: FeedContentOnTheFlyResponse) => void = () => {};
			const firstCallPromise = new Promise<FeedContentOnTheFlyResponse>(
				(resolve) => {
					resolveFirst = resolve;
				},
			);

			mockedGetContent.mockImplementationOnce(() => firstCallPromise);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Second</p>",
				article_id: "art-2",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://zenn.dev/article-1"),
				makeFeed("2", "https://zenn.dev/article-2"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);

			// Advance past both PREFETCH_DELAY windows.
			await vi.advanceTimersByTimeAsync(1100);

			// Only the first call should have started — the second is queued
			// behind the first promise on the same host.
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
			expect(mockedGetContent).toHaveBeenNthCalledWith(
				1,
				"https://zenn.dev/article-1",
			);

			// Resolve first → second runs.
			resolveFirst({
				content: "<p>First</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-2")).toBe(
					"<p>Second</p>",
				);
			});

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
			expect(mockedGetContent).toHaveBeenNthCalledWith(
				2,
				"https://zenn.dev/article-2",
			);
		});

		it("allows prefetch for different hosts concurrently", async () => {
			mockedGetContent
				.mockResolvedValueOnce({
					content: "<p>Zenn</p>",
					article_id: "art-z",
					og_image_url: null,
				} as unknown as FeedContentOnTheFlyResponse)
				.mockResolvedValueOnce({
					content: "<p>Example</p>",
					article_id: "art-e",
					og_image_url: null,
				} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://active.com/page"),
				makeFeed("1", "https://zenn.dev/article-1"),
				makeFeed("2", "https://example.com/article-2"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);

			// Use async timer advancement to flush microtasks between timer steps
			await vi.advanceTimersByTimeAsync(1100);

			// Wait for both fetches to fully complete (cache populated)
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-1")).toBe(
					"<p>Zenn</p>",
				);
				expect(
					prefetcher.getCachedContent("https://example.com/article-2"),
				).toBe("<p>Example</p>");
			});

			// Both calls should have been made (different hosts)
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});
	});

	describe("429 cooldown", () => {
		it("applies a 30s cooldown to a host after ConnectError ResourceExhausted", async () => {
			// First call rejects with 429; subsequent prefetches on the same host
			// must skip until the cooldown lifts.
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);

			await vi.advanceTimersByTimeAsync(1_100);
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(1);
			});

			// Re-trigger before cooldown expires — must NOT call client again.
			await vi.advanceTimersByTimeAsync(15_000);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(2_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Past 30s — cooldown lifted, prefetch resumes.
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>OK</p>",
				article_id: "art-ok",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await vi.advanceTimersByTimeAsync(20_000);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(2);
			});
		});

		it("cooldown is per-host: a 429 on host A does not block host B", async () => {
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>B</p>",
				article_id: "art-b",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://active.com/page"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-b")).toBe(
					"<p>B</p>",
				);
			});

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("honors Retry-After header (delta-seconds) when shorter than the default 30s", async () => {
			const meta = new Headers({ "Retry-After": "2" });
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted, meta),
			);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>OK</p>",
				article_id: "art-ok",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			// Wait through Retry-After window and re-trigger.
			await vi.advanceTimersByTimeAsync(2_500);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
					"<p>OK</p>",
				);
			});
		});

		it("seedCache bypasses cooldown (manual path is unaffected)", async () => {
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(1);
			});

			// Manual seed during cooldown still populates the cache.
			prefetcher.seedCache(
				"https://dev.to/article-1",
				"<p>Manual</p>",
				"art-m",
				null,
			);

			expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
				"<p>Manual</p>",
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
			} as unknown as FeedContentOnTheFlyResponse);

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
			} as unknown as FeedContentOnTheFlyResponse);

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
			} as unknown as FeedContentOnTheFlyResponse);

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
