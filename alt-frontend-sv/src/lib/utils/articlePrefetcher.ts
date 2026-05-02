import { Code, ConnectError } from "@connectrpc/connect";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { RenderFeed } from "$lib/schema/feed";
import { parseRetryAfter } from "./retryAfter";

const MAX_CACHE_SIZE = 30;
const PREFETCH_DELAY = 500; // ms
const DISMISSED_CLEANUP_DELAY = 3000; // ms
// Default cooldown applied to a host after a 429 / ResourceExhausted.
// ADR-000884: matches the typical reset window of the backend host rate limiter
// (5 rps). When the server returns Retry-After, that value wins.
const HOST_COOLDOWN_MS = 30_000;

export class ArticlePrefetcher {
	private contentCache = new Map<string, string | "loading">();
	private articleIdCache = new Map<string, string>();
	private ogImageCache = new Map<string, string | null>();
	private prefetchTimeouts: ReturnType<typeof setTimeout>[] = [];
	private dismissedArticles = new Set<string>();
	private dismissalTimeouts = new Map<string, ReturnType<typeof setTimeout>>();
	// Per-host promise chain: a new prefetch on the same host awaits the
	// previous one before issuing the actual HTTP call. Serialization, not skip.
	private hostInflight = new Map<string, Promise<void>>();
	// Per-host cooldown (epoch ms when the cooldown ends). Set on 429.
	private hostCooldown = new Map<string, number>();
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
	private prefetchContent(feed: RenderFeed): Promise<void> {
		const cacheKey = feed.normalizedUrl;

		if (!cacheKey) return Promise.resolve();
		if (this.dismissedArticles.has(cacheKey)) return Promise.resolve();
		if (this.contentCache.has(cacheKey)) return Promise.resolve();

		let host: string;
		try {
			host = new URL(cacheKey).host;
		} catch {
			return Promise.resolve();
		}

		// Honor cooldown: if this host returned 429 recently, skip until it lifts.
		const cooldownUntil = this.hostCooldown.get(host);
		if (cooldownUntil !== undefined) {
			if (Date.now() < cooldownUntil) return Promise.resolve();
			this.hostCooldown.delete(host);
		}

		// Serialize per-host: chain after any in-flight prefetch on this host.
		const previous = this.hostInflight.get(host) ?? Promise.resolve();
		const next = previous.then(() => this.runPrefetch(cacheKey, host));
		this.hostInflight.set(host, next);
		void next.finally(() => {
			if (this.hostInflight.get(host) === next) {
				this.hostInflight.delete(host);
			}
		});
		return next;
	}

	private async runPrefetch(cacheKey: string, host: string): Promise<void> {
		// A cooldown may have been set by a peer chained behind us — re-check
		// once the chain reaches our turn so we do not issue a doomed call.
		const cooldownUntil = this.hostCooldown.get(host);
		if (cooldownUntil !== undefined && Date.now() < cooldownUntil) return;
		// Another path may have populated the cache while we waited.
		if (this.contentCache.has(cacheKey)) return;

		try {
			this.contentCache.set(cacheKey, "loading");

			const response = await getFeedContentOnTheFlyClient(cacheKey);

			if (response.content) {
				this.contentCache.set(cacheKey, response.content);
				this.onContentFetched?.(cacheKey, response.content);
			} else {
				this.contentCache.delete(cacheKey);
			}

			if (response.article_id) {
				this.articleIdCache.set(cacheKey, response.article_id);
				this.onArticleIdCached?.(cacheKey, response.article_id);
			}

			this.ogImageCache.set(cacheKey, response.og_image_url || null);
			this.onOgImageFetched?.();

			this.evictOldEntries();
		} catch (error) {
			this.contentCache.delete(cacheKey);
			const connectErr = ConnectError.from(error);
			if (connectErr.code === Code.ResourceExhausted) {
				const retryAfterMs = parseRetryAfter(
					connectErr.metadata.get("Retry-After"),
				);
				const cooldown = retryAfterMs ?? HOST_COOLDOWN_MS;
				this.hostCooldown.set(host, Date.now() + cooldown);
			}
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
