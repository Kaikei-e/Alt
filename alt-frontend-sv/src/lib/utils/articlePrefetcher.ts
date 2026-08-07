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
	// Keyed by cache key, not a flat list: re-arming a timer that is still
	// wanted restarts its delay, and the driving $effect fires more often than
	// PREFETCH_DELAY, which starved the far lookahead slots entirely.
	private prefetchTimeouts = new Map<string, ReturnType<typeof setTimeout>>();
	private dismissedArticles = new Set<string>();
	private dismissalTimeouts = new Map<string, ReturnType<typeof setTimeout>>();
	// Per-host promise chain: a new prefetch on the same host awaits the
	// previous one before issuing the actual HTTP call. Serialization, not skip.
	private hostInflight = new Map<string, Promise<void>>();
	// Per-URL in-flight fetches, so the visible card joins the prefetch already
	// running for its own URL instead of racing it into the host rate limiter.
	private inflight = new Map<string, Promise<string | null>>();
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
		if (this.hasBodyOrPending(cacheKey)) return Promise.resolve();

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
		const next = previous.then(() =>
			this.fetchContent(cacheKey, host).then(() => undefined),
		);
		this.hostInflight.set(host, next);
		void next.finally(() => {
			if (this.hostInflight.get(host) === next) {
				this.hostInflight.delete(host);
			}
		});
		return next;
	}

	/**
	 * Fetch the body for a URL the reader is looking at right now.
	 *
	 * Cards used to call the RPC directly on mount. The prefetcher's "loading"
	 * sentinel reads as a cache miss, so a card that mounted mid-prefetch fired
	 * a second request for the same URL — unserialized, ignoring the host
	 * cooldown, and doubling the load the host rate limiter sees. Routing the
	 * card through here makes that a no-op join.
	 *
	 * Resolves to null when the body is unavailable right now (empty response,
	 * failed request, or an active host cooldown). Null is retryable: callers
	 * should offer the reader a retry rather than latching an error.
	 */
	public ensureContent(feedUrl: string): Promise<string | null> {
		if (!feedUrl) return Promise.resolve(null);

		const cached = this.readBody(feedUrl);
		if (cached) return Promise.resolve(cached);

		const pending = this.inflight.get(feedUrl);
		if (pending) return pending;

		let host: string;
		try {
			host = new URL(feedUrl).host;
		} catch {
			return Promise.resolve(null);
		}

		const cooldownUntil = this.hostCooldown.get(host);
		if (cooldownUntil !== undefined) {
			if (Date.now() < cooldownUntil) return Promise.resolve(null);
			this.hostCooldown.delete(host);
		}

		// The visible card does not queue behind the lookahead prefetches — it
		// is what the reader is waiting on. It still registers on the host chain
		// so those prefetches fall in behind it instead of racing it.
		const run = this.fetchContent(feedUrl, host);
		const gate = run.then(
			() => undefined,
			() => undefined,
		);
		const previous = this.hostInflight.get(host);
		const chained = previous ? previous.then(() => gate) : gate;
		this.hostInflight.set(host, chained);
		void chained.finally(() => {
			if (this.hostInflight.get(host) === chained) {
				this.hostInflight.delete(host);
			}
		});

		return run;
	}

	/** Single in-flight request per URL, shared by prefetch and visible card. */
	private fetchContent(cacheKey: string, host: string): Promise<string | null> {
		const pending = this.inflight.get(cacheKey);
		if (pending) return pending;

		const run = this.runFetch(cacheKey, host);
		this.inflight.set(cacheKey, run);
		void run.finally(() => {
			if (this.inflight.get(cacheKey) === run) {
				this.inflight.delete(cacheKey);
			}
		});
		return run;
	}

	private async runFetch(
		cacheKey: string,
		host: string,
	): Promise<string | null> {
		// A cooldown may have been set by a peer chained behind us — re-check
		// once the chain reaches our turn so we do not issue a doomed call.
		const cooldownUntil = this.hostCooldown.get(host);
		if (cooldownUntil !== undefined && Date.now() < cooldownUntil) return null;
		// Another path may have populated the cache while we waited.
		const cached = this.readBody(cacheKey);
		if (cached) return cached;

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
			return response.content || null;
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
			return null;
		}
	}

	/** A real body, or null for "absent", "empty" and the loading sentinel. */
	private readBody(cacheKey: string): string | null {
		const cached = this.contentCache.get(cacheKey);
		if (cached === undefined || cached === "loading" || cached === "") {
			return null;
		}
		return cached;
	}

	/**
	 * Whether a fetch would be redundant. Deliberately not `contentCache.has()`:
	 * an empty-string entry answered `has()` with true and `readBody()` with
	 * null, which permanently suppressed prefetch for a URL nothing had fetched.
	 */
	private hasBodyOrPending(cacheKey: string): boolean {
		const cached = this.contentCache.get(cacheKey);
		return cached === "loading" || (cached !== undefined && cached !== "");
	}

	/**
	 * Trigger prefetch for next N articles
	 */
	public triggerPrefetch(
		feeds: RenderFeed[],
		activeIndex: number,
		prefetchAhead: number = 2,
	) {
		const wanted = new Set<string>();
		for (let i = 1; i <= prefetchAhead; i++) {
			const key = feeds[activeIndex + i]?.normalizedUrl;
			if (key) wanted.add(key);
		}

		// Cancel only the timers whose feed left the lookahead window. Clearing
		// every timer on each call restarted the 500ms ladder from zero, and the
		// caller re-runs this more often than that, so the far slots never fired.
		for (const [key, timeout] of this.prefetchTimeouts) {
			if (!wanted.has(key)) {
				clearTimeout(timeout);
				this.prefetchTimeouts.delete(key);
			}
		}

		// Prefetch next N articles
		for (let i = 1; i <= prefetchAhead; i++) {
			const nextFeed = feeds[activeIndex + i];
			const cacheKey = nextFeed?.normalizedUrl;
			if (!nextFeed || !cacheKey) continue;
			if (this.prefetchTimeouts.has(cacheKey)) continue;
			if (this.hasBodyOrPending(cacheKey)) continue;

			const timeout = setTimeout(() => {
				this.prefetchTimeouts.delete(cacheKey);
				void this.prefetchContent(nextFeed);
			}, PREFETCH_DELAY * i);
			this.prefetchTimeouts.set(cacheKey, timeout);
		}
	}

	/**
	 * Get cached content for a feed URL
	 */
	public getCachedContent(feedUrl: string): string | null {
		return this.readBody(feedUrl);
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
		if (!feedUrl) return;

		// Image-only seeds pass "" for the body. Writing that entry made
		// `has()` report a hit — suppressing every future prefetch for the URL —
		// while every reader still saw a miss, so the card fell back to the
		// summary for the rest of the session. Only a real body is cacheable.
		if (content) {
			this.contentCache.set(feedUrl, content);
		}
		if (articleId) {
			this.articleIdCache.set(feedUrl, articleId);
		}
		this.ogImageCache.set(feedUrl, ogImageProxyUrl || ogImageUrl);
		this.onOgImageFetched?.();
		this.evictOldEntries();
	}

	private evictOldEntries(): void {
		if (this.contentCache.size <= MAX_CACHE_SIZE) return;

		for (const key of [...this.contentCache.keys()]) {
			if (this.contentCache.size <= MAX_CACHE_SIZE) break;
			// Never drop the "loading" sentinel of a fetch still in flight:
			// the next caller would see a miss and issue a duplicate request.
			if (this.contentCache.get(key) === "loading") continue;
			this.contentCache.delete(key);
			this.ogImageCache.delete(key);
			// The article id belongs to the body it was extracted from; leaving
			// it behind grew articleIdCache without bound.
			this.articleIdCache.delete(key);
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
