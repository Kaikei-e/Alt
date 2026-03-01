import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { RenderFeed } from "$lib/schema/feed";

const MAX_CACHE_SIZE = 10;
const PREFETCH_DELAY = 500; // ms
const DISMISSED_CLEANUP_DELAY = 3000; // ms

export class ArticlePrefetcher {
	private contentCache = new Map<string, string | "loading">();
	private articleIdCache = new Map<string, string>();
	private ogImageCache = new Map<string, string | null>();
	private prefetchTimeouts: ReturnType<typeof setTimeout>[] = [];
	private dismissedArticles = new Set<string>();
	private dismissalTimeouts = new Map<string, ReturnType<typeof setTimeout>>();
	private onContentFetched:
		| ((feedUrl: string, content: string) => void)
		| null = null;
	private onOgImageFetched: (() => void) | null = null;
	private onArticleIdCached:
		| ((feedUrl: string, articleId: string) => void)
		| null = null;

	public setOnContentFetched(
		cb: ((feedUrl: string, content: string) => void) | null,
	): void {
		this.onContentFetched = cb;
	}

	public setOnOgImageFetched(cb: (() => void) | null): void {
		this.onOgImageFetched = cb;
	}

	public setOnArticleIdCached(
		cb: ((feedUrl: string, articleId: string) => void) | null,
	): void {
		this.onArticleIdCached = cb;
	}

	/**
	 * Prefetch content for a single article
	 * Uses normalizedUrl as cache key for consistency with FeedDetailModal
	 */
	private async prefetchContent(feed: RenderFeed) {
		const cacheKey = feed.normalizedUrl;

		// Skip feeds with empty URL
		if (!cacheKey) return;

		// Skip if article is being dismissed
		if (this.dismissedArticles.has(cacheKey)) {
			console.log(
				`[ArticlePrefetcher] Skipping dismissed article: ${cacheKey}`,
			);
			return;
		}

		// Skip if already in cache
		if (this.contentCache.has(cacheKey)) {
			return;
		}

		try {
			// Mark as loading
			this.contentCache.set(cacheKey, "loading");

			// Fetch content using normalizedUrl for consistent caching
			const response = await getFeedContentOnTheFlyClient(cacheKey);

			if (response.content) {
				this.contentCache.set(cacheKey, response.content);
				this.onContentFetched?.(cacheKey, response.content);
			} else {
				this.contentCache.delete(cacheKey);
			}

			// Cache article_id if present and notify listeners
			if (response.article_id) {
				this.articleIdCache.set(cacheKey, response.article_id);
				this.onArticleIdCached?.(cacheKey, response.article_id);
			}

			// Cache raw og_image_url; proxy URL comes from BatchPrefetchImages
			this.ogImageCache.set(
				cacheKey,
				response.og_image_url || null,
			);
			this.onOgImageFetched?.();

			this.evictOldEntries();
		} catch (error) {
			this.contentCache.delete(cacheKey);
			console.warn(
				`[ArticlePrefetcher] Failed to prefetch content: ${cacheKey}`,
				error,
			);
		}
	}

	/**
	 * Trigger prefetch for next N articles
	 */
	public triggerPrefetch(
		feeds: RenderFeed[],
		activeIndex: number,
		prefetchAhead: number = 2,
	) {
		// Clear pending timeouts
		this.prefetchTimeouts.forEach((timeout) => {
			clearTimeout(timeout);
		});
		this.prefetchTimeouts = [];

		// Prefetch next N articles
		for (let i = 1; i <= prefetchAhead; i++) {
			const nextFeed = feeds[activeIndex + i];
			if (nextFeed) {
				const timeout = setTimeout(() => {
					void this.prefetchContent(nextFeed);
				}, PREFETCH_DELAY * i);
				this.prefetchTimeouts.push(timeout);
			}
		}
	}

	/**
	 * Get cached content for a feed URL
	 */
	public getCachedContent(feedUrl: string): string | null {
		const cached = this.contentCache.get(feedUrl);
		return cached === "loading" ? null : cached || null;
	}

	/**
	 * Get cached article_id for a feed URL
	 */
	public getCachedArticleId(feedUrl: string): string | null {
		return this.articleIdCache.get(feedUrl) ?? null;
	}

	/**
	 * Get cached og:image URL for a feed URL
	 */
	public getCachedOgImage(feedUrl: string): string | null {
		return this.ogImageCache.get(feedUrl) ?? null;
	}

	/**
	 * Seed cache directly without fetching from API.
	 * Used by SwipeFeedScreen to cache the first feed's content from loadMore.
	 */
	public seedCache(
		feedUrl: string,
		content: string,
		articleId: string,
		ogImageUrl: string | null,
		ogImageProxyUrl?: string | null,
	): void {
		this.contentCache.set(feedUrl, content);
		this.articleIdCache.set(feedUrl, articleId);
		this.ogImageCache.set(feedUrl, ogImageProxyUrl || ogImageUrl);
		this.onOgImageFetched?.();
		this.evictOldEntries();
	}

	private evictOldEntries(): void {
		while (this.contentCache.size > MAX_CACHE_SIZE) {
			const oldestKey = this.contentCache.keys().next().value;
			if (oldestKey !== undefined) {
				this.contentCache.delete(oldestKey);
				this.ogImageCache.delete(oldestKey);
			}
		}
	}

	/**
	 * Mark an article as dismissed
	 */
	public markAsDismissed(feedUrl: string) {
		this.dismissedArticles.add(feedUrl);

		const existingTimeout = this.dismissalTimeouts.get(feedUrl);
		if (existingTimeout) {
			clearTimeout(existingTimeout);
		}

		const timeout = setTimeout(() => {
			this.dismissedArticles.delete(feedUrl);
			this.dismissalTimeouts.delete(feedUrl);
		}, DISMISSED_CLEANUP_DELAY);

		this.dismissalTimeouts.set(feedUrl, timeout);
	}
}

export const articlePrefetcher = new ArticlePrefetcher();
