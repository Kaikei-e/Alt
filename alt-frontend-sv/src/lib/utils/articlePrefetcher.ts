import { Code, ConnectError } from "@connectrpc/connect";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { RenderFeed } from "$lib/schema/feed";
import { isGlobalFailureScope } from "./errorClassification";
import { parseRetryAfter } from "./retryAfter";

const MAX_CACHE_SIZE = 30;
const PREFETCH_DELAY = 500; // ms
const DISMISSED_CLEANUP_DELAY = 3000; // ms
// Default cooldown applied to a host after a 429 / ResourceExhausted.
// ADR-000884: matches the typical reset window of the backend host rate limiter
// (5 rps). When the server returns Retry-After, that value wins.
const HOST_COOLDOWN_MS = 30_000;
// Pause applied to every host after a failure that declares itself global —
// the BFF's circuit breaker being open, which no other host can route around.
// The breaker's own open timeout bounds this and travels in Retry-After; the
// default only covers a gateway that declared the scope but not the timing.
//
// It is armed by the declared scope, never by CodeUnavailable alone. Both a
// dead publisher and an open breaker arrive as that code, and treating the
// former as the latter blacked the reader out for 30s per broken link.
const GLOBAL_COOLDOWN_MS = 30_000;
// How often a request the reader is waiting on may cross an active cooldown.
//
// Cooldowns exist to keep the lookahead ladder from walking into a gate that
// already refused it. The visible card is not the ladder: it is one request the
// reader explicitly asked for, and refusing it without sending anything is what
// made a few seconds of backoff read as a permanently broken card — retry
// button included, since that took the same short circuit. Rationing rather
// than allowing keeps a held retry button from becoming the hammering the
// cooldown was installed to prevent. The interval matches the BFF's
// external-content open timeout, so a probe lands about as often as the
// gateway re-probes itself.
const FOREGROUND_PROBE_INTERVAL_MS = 5_000;
// Probe-ledger key for the all-hosts pause. Not a possible host name.
const GLOBAL_SCOPE_KEY = "*";

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
	// Per-host cooldown (epoch ms when the cooldown ends). Set on 429, and on
	// any Unavailable that did not declare itself global.
	private hostCooldown = new Map<string, number>();
	// Epoch ms when the all-hosts pause ends, 0 when there is none. Set only by
	// a failure carrying an explicit global scope.
	private globalCooldownUntil = 0;
	// When each cooldown scope last let a foreground request through, keyed by
	// host or by GLOBAL_SCOPE_KEY. Rations the reader's attempts without
	// silencing them.
	private foregroundProbeAt = new Map<string, number>();
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

	/** Milliseconds left on the all-hosts pause, 0 when it is not active. */
	private globalPauseRemaining(): number {
		if (this.globalCooldownUntil === 0) return 0;
		const remaining = this.globalCooldownUntil - Date.now();
		if (remaining > 0) return remaining;
		this.globalCooldownUntil = 0;
		return 0;
	}

	/** Milliseconds left on this host's cooldown, 0 when it is not active. */
	private hostPauseRemaining(host: string): number {
		const until = this.hostCooldown.get(host);
		if (until === undefined) return 0;
		const remaining = until - Date.now();
		if (remaining > 0) return remaining;
		this.hostCooldown.delete(host);
		return 0;
	}

	/**
	 * The cooldown scope covering this host right now, or null when none does.
	 * The all-hosts pause wins: backing off one host cannot route around it.
	 */
	private activeCooldownScope(host: string): string | null {
		if (this.globalPauseRemaining() > 0) return GLOBAL_SCOPE_KEY;
		if (this.hostPauseRemaining(host) > 0) return host;
		return null;
	}

	/**
	 * Whether a foreground request may cross this scope's cooldown now,
	 * consuming the slot when it may. Callers that are told no must not fetch.
	 */
	private claimForegroundProbe(scope: string): boolean {
		const now = Date.now();
		const last = this.foregroundProbeAt.get(scope);
		if (last !== undefined && now - last < FOREGROUND_PROBE_INTERVAL_MS) {
			return false;
		}
		this.foregroundProbeAt.set(scope, now);
		return true;
	}

	/**
	 * Drop the backoff after a request completes. Something got through, so
	 * whatever the cooldown was guarding is over; sitting out the rest of the
	 * window would keep the ladder suppressed and the next cards blank long
	 * after the host or the breaker recovered.
	 */
	private clearCooldowns(host: string): void {
		this.hostCooldown.delete(host);
		this.globalCooldownUntil = 0;
		this.foregroundProbeAt.delete(host);
		this.foregroundProbeAt.delete(GLOBAL_SCOPE_KEY);
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

		// Honor the cooldown outright. The lookahead has nobody waiting on it,
		// so unlike the visible card it gets no probe: skip until it lifts.
		if (this.hostPauseRemaining(host) > 0) return Promise.resolve();

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
	 * failed request, or a cooldown that has already spent its probe). Null is
	 * retryable: callers should offer the reader a retry rather than latching
	 * an error.
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

		// A cooldown covering this card does not silence it. The reader asked
		// for this one body, so they get an attempt — rationed, not refused.
		const scope = this.activeCooldownScope(host);
		if (scope !== null && !this.claimForegroundProbe(scope)) {
			return Promise.resolve(null);
		}

		// The visible card does not queue behind the lookahead prefetches — it
		// is what the reader is waiting on. It still registers on the host chain
		// so those prefetches fall in behind it instead of racing it.
		const run = this.fetchContent(feedUrl, host, scope !== null);
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
	private fetchContent(
		cacheKey: string,
		host: string,
		crossCooldown = false,
	): Promise<string | null> {
		const pending = this.inflight.get(cacheKey);
		if (pending) return pending;

		const run = this.runFetch(cacheKey, host, crossCooldown);
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
		crossCooldown: boolean,
	): Promise<string | null> {
		// A cooldown may have been set by a peer chained behind us — re-check
		// once the chain reaches our turn so we do not issue a doomed call.
		// A caller holding a foreground probe has already been granted its one
		// crossing; re-checking here would revoke it and put the reader back on
		// a card that fails without asking anything.
		if (!crossCooldown) {
			if (this.globalPauseRemaining() > 0) return null;
			if (this.hostPauseRemaining(host) > 0) return null;
		}
		// Another path may have populated the cache while we waited.
		const cached = this.readBody(cacheKey);
		if (cached) return cached;

		try {
			this.contentCache.set(cacheKey, "loading");

			const response = await getFeedContentOnTheFlyClient(cacheKey);
			this.clearCooldowns(host);

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
			const retryAfterMs = parseRetryAfter(
				connectErr.metadata.get("Retry-After"),
			);
			if (connectErr.code === Code.ResourceExhausted) {
				this.hostCooldown.set(
					host,
					Date.now() + (retryAfterMs ?? HOST_COOLDOWN_MS),
				);
			} else if (isGlobalFailureScope(connectErr.metadata)) {
				this.globalCooldownUntil =
					Date.now() + (retryAfterMs ?? GLOBAL_COOLDOWN_MS);
			} else if (connectErr.code === Code.Unavailable) {
				// Unavailable that did not claim to be global. alt-backend
				// returns it for a publisher that never answered, which is one
				// host's problem — reading every one of them as an open breaker
				// paused the whole reader on a single dead link.
				this.hostCooldown.set(
					host,
					Date.now() + (retryAfterMs ?? HOST_COOLDOWN_MS),
				);
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

			this.schedulePrefetch(nextFeed, cacheKey, PREFETCH_DELAY * i);
		}
	}

	private schedulePrefetch(
		feed: RenderFeed,
		cacheKey: string,
		delay: number,
	): void {
		const timeout = setTimeout(() => {
			this.prefetchTimeouts.delete(cacheKey);

			// The breaker is open for every host, so this rung of the ladder
			// cannot succeed. Re-arm rather than drop it: dropping left the
			// queued articles unfetched until the reader happened to swipe again,
			// which is what latched their cards into the error state.
			const pause = this.globalPauseRemaining();
			if (pause > 0) {
				this.schedulePrefetch(feed, cacheKey, pause + PREFETCH_DELAY);
				return;
			}

			void this.prefetchContent(feed);
		}, delay);
		this.prefetchTimeouts.set(cacheKey, timeout);
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
