import { Code, ConnectError } from "@connectrpc/connect";
import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import { batchPrefetchArticleContentClient } from "$lib/api/client/articles";
import type { AltFetchPriority } from "$lib/connect/transport-client";
import type { RenderFeed } from "$lib/schema/feed";
import {
	isGlobalFailureScope,
	isRetryableContentError,
} from "./errorClassification";
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
// Retries the background ladder may spend on one article, on top of the
// original attempt. Google SRE budgets three attempts per request; the first
// try is one of them, so the client's share is two. Retries also multiply
// along browser -> BFF -> alt-backend -> publisher, and a generous per-hop
// budget becomes a multiplicative one at the far end.
const MAX_PREFETCH_RETRIES = 2;
// Full Jitter parameters (Brooker, AWS Architecture Blog 2015):
// delay = random(0, min(RETRY_MAX_DELAY_MS, RETRY_BASE_DELAY_MS * 2**attempt)).
// The randomness is the point. Deterministic backoff puts every client that
// failed in the same incident back on the wire at the same instant, which is
// how a recovering host gets knocked over by the recovery.
const RETRY_BASE_DELAY_MS = 1_000;
const RETRY_MAX_DELAY_MS = 30_000;
// Ceiling on a server-supplied Retry-After. The values Alt actually issues are
// the breaker's open timeout (5s) and the host-slot wait, so anything past a
// minute is a bug, a stale HTTP-date, or a misconfigured upstream — and
// honouring it literally blacks the host out for the rest of the reading
// session. The clamp only shortens a client-side wait; the backend keeps its
// own politeness budget either way, so nothing about outbound rate is relaxed.
const RETRY_AFTER_MAX_MS = 60_000;
// Lookahead slots the per-URL ladder keeps for itself. Everything past this is
// handed to the server's BatchPrefetchArticleContent warm instead.
//
// The split exists because the two paths are not additive. A warm returns no
// content, so it can never fill this client's cache; a per-URL fetch of a URL
// the server is warming right now meets that warm at alt-backend's *shared*
// scraping-policy gate — PrefetchArticle is a second usecase over the same
// policy port, so the two have separate singleflight groups and only the
// crawl-delay in common. Whichever arrives second is told the delay has not
// elapsed, which reaches this class as a 429 and arms a host cooldown. So the
// sets are disjoint: the two nearest cards, the ones the reader will actually
// reach, stay with the ladder that fills the cache, and the far slots go to
// the server, which can run them in parallel across hosts and shed rather than
// queue. prefetchAhead itself is untouched — ADR-000884 cut it 10 -> 4 and
// nothing here reopens that.
const LADDER_OWNED_AHEAD = 2;
// Ceiling on one warm batch. The server takes at most 5 and drops the rest, so
// a longer list is only bytes.
const MAX_WARM_URLS = 5;
// Floor on the interval between warm batches.
//
// The driving $effect re-runs on every swipe and on every `feeds`
// reassignment, which is far more often than a warm is worth: the server sheds
// rather than queues, so an unbounded client would be paying BFF and backend
// cost to be refused. Matches CLAUDE.md's 5-second minimum between external
// API calls, which is the budget this is spending a slice of.
const WARM_MIN_INTERVAL_MS = 5_000;
// How long a warmed article stays off the per-URL ladder.
//
// This is what makes "no double fetch" a guarantee rather than a hope. The
// warm's own life is a store probe, a bounded host-slot wait and one fetch
// capped at the usecase's 8s external budget, so after this window it has
// either stored the body — making the ladder's later fetch a database read —
// or given up, leaving the ladder to do the one real fetch. Either way the
// publisher sees one request. The ladder re-arms rather than drops, the same
// way it does for a cooldown that appeared underneath it.
const WARM_SETTLE_MS = 10_000;
// Cap on the two warm ledgers. They are session-scoped bookkeeping, not a
// cache, and an unbounded Set on a surface designed for endless swiping grows
// for as long as the tab is open.
const MAX_WARM_LEDGER = 120;

/** Host of a cache key, or null when it is not a usable absolute URL. */
function hostOf(cacheKey: string): string | null {
	try {
		return new URL(cacheKey).host;
	} catch {
		return null;
	}
}

/** What the ledger remembers about an article the ladder failed to fetch. */
interface RetryLedgerEntry {
	/** Retries already spent. Never exceeds MAX_PREFETCH_RETRIES. */
	attempts: number;
	/** Epoch ms the next retry is booked for. */
	nextAt: number;
}

export class ArticlePrefetcher {
	private contentCache = new Map<string, string | "loading">();
	private articleIdCache = new Map<string, string>();
	/**
	 * feed URL -> the *signed* og:image proxy path, or null for "nothing to
	 * render".
	 *
	 * Never a publisher's own URL. What this cache holds is read straight into
	 * an `<img src>` by the swipe surface, and a raw cross-origin URL there is
	 * an unproxied request that skips the rate limiting, SSRF validation,
	 * domain allow-list and re-encoding `/v1/images/proxy` exists to apply.
	 */
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
	// Retries booked for articles the ladder failed to fetch, keyed by cache
	// key. Bounded by the lookahead window: triggerPrefetch drops every entry
	// whose article has left it.
	private retryLedger = new Map<string, RetryLedgerEntry>();
	// The articles currently occupying the lookahead window, by cache key.
	// A retry is only worth booking for one the reader is still heading
	// towards, and this is also what keeps the ledger from growing: the
	// foreground card is never in here, so its failures book nothing.
	private windowFeeds = new Map<string, RenderFeed>();
	// Articles this client has already asked the server to warm. One attempt
	// per URL, ever: the server does not retry and the client must not poll,
	// because browser -> BFF -> alt-backend -> publisher is four hops and a
	// retry at each multiplies rather than adds.
	private warmAttempted = new Set<string>();
	// Epoch ms until which a warmed article is off the per-URL ladder. Dropped
	// as soon as the batch is refused: nothing was warmed, so nothing is owed.
	private warmSettleUntil = new Map<string, number>();
	// Epoch ms of the last warm batch, for WARM_MIN_INTERVAL_MS.
	private lastWarmAt = 0;
	// Latched by a FAILED_PRECONDITION: the capability is off by configuration
	// and no retry can change that. Only the warm stops; the per-URL ladder is
	// untouched and keeps working on its own.
	private warmDisabled = false;
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
	 * Clamp a server-supplied Retry-After to something a reader can sit out.
	 * Absent or unparseable stays null so callers fall back to their default.
	 */
	private clampRetryAfter(value: number | null): number | null {
		if (value === null) return null;
		if (value <= 0) return 0;
		return Math.min(value, RETRY_AFTER_MAX_MS);
	}

	/**
	 * How long to wait before retrying, in ms.
	 *
	 * Full Jitter over the exponential window, laid on top of a floor: the
	 * server's own Retry-After when it sent one, and whatever cooldown is
	 * currently in force. The floor is what keeps a retry from firing inside a
	 * gate that has already refused this host — the guarantee ADR-000884's
	 * per-host cooldown exists to give — while the jitter is what keeps every
	 * client that failed together from coming back together.
	 */
	private retryDelayMs(
		attempt: number,
		retryAfterMs: number | null,
		host: string,
	): number {
		const jitterWindow = Math.min(
			RETRY_MAX_DELAY_MS,
			RETRY_BASE_DELAY_MS * 2 ** attempt,
		);
		const jitter = Math.floor(Math.random() * (jitterWindow + 1));
		const floor = Math.max(
			retryAfterMs ?? 0,
			this.globalPauseRemaining(),
			this.hostPauseRemaining(host),
		);
		// Never sooner than one rung of the ladder: a DeadlineExceeded sets no
		// cooldown at all, and an unfloored zero-jitter roll would put the
		// retry on the wire in the same tick as the failure.
		return Math.max(floor + jitter, PREFETCH_DELAY);
	}

	/**
	 * Book a retry for an article the lookahead ladder failed to fetch.
	 *
	 * Only for articles still inside the lookahead window, whichever path
	 * discovered the failure. The card the reader is looking at sits at
	 * activeIndex and is never in that window: it fails through ensureContent,
	 * which already has the rationed foreground probe and the reader's own
	 * retry button, and a background retry behind those would be a second,
	 * invisible request for the same body.
	 */
	private armRetry(
		cacheKey: string,
		host: string,
		error: ConnectError,
		retryAfterMs: number | null,
	): void {
		if (!isRetryableContentError(error)) {
			// Permanent for this article: whatever is in the ledger from an
			// earlier, different failure is no longer worth acting on.
			this.retryLedger.delete(cacheKey);
			return;
		}

		const feed = this.windowFeeds.get(cacheKey);
		if (!feed) return;

		const existing = this.retryLedger.get(cacheKey);
		// A retry is already booked. Two paths can fail the same URL — the
		// ladder and the card that mounted on top of it — and the second one
		// must not stack a timer or spend an attempt the ladder still owns.
		if (existing && existing.nextAt > Date.now()) return;

		const attempts = (existing?.attempts ?? 0) + 1;
		if (attempts > MAX_PREFETCH_RETRIES) {
			this.retryLedger.delete(cacheKey);
			return;
		}

		const delay = this.retryDelayMs(attempts, retryAfterMs, host);
		this.retryLedger.set(cacheKey, { attempts, nextAt: Date.now() + delay });
		this.schedulePrefetch(feed, cacheKey, delay);
	}

	/** Milliseconds a warmed article still owes the ladder, 0 when none. */
	private warmSettleRemaining(cacheKey: string): number {
		const until = this.warmSettleUntil.get(cacheKey);
		if (until === undefined) return 0;
		const remaining = until - Date.now();
		if (remaining > 0) return remaining;
		this.warmSettleUntil.delete(cacheKey);
		return 0;
	}

	/**
	 * Record a warm in both ledgers, oldest-first eviction on each.
	 *
	 * `warmSettleUntil` entries expire by time, but only when something asks
	 * about that key — and an article the reader swiped past is never asked
	 * about again. On a surface built for endless swiping that is a leak, so
	 * the cap is what actually bounds it.
	 */
	private rememberWarm(cacheKey: string, settleUntil: number): void {
		this.warmAttempted.delete(cacheKey);
		this.warmAttempted.add(cacheKey);
		while (this.warmAttempted.size > MAX_WARM_LEDGER) {
			const oldest = this.warmAttempted.values().next().value;
			if (oldest === undefined) break;
			this.warmAttempted.delete(oldest);
		}

		this.warmSettleUntil.delete(cacheKey);
		this.warmSettleUntil.set(cacheKey, settleUntil);
		while (this.warmSettleUntil.size > MAX_WARM_LEDGER) {
			const oldest = this.warmSettleUntil.keys().next().value;
			if (oldest === undefined) break;
			this.warmSettleUntil.delete(oldest);
		}
	}

	/**
	 * The window's far slots that may be handed to the server this pass.
	 *
	 * Everything excluded here stays with the per-URL ladder, so a refusal
	 * costs pacing, never coverage.
	 */
	private selectWarmUrls(windowOrder: [string, RenderFeed][]): string[] {
		if (this.warmDisabled) return [];
		if (windowOrder.length <= LADDER_OWNED_AHEAD) return [];
		// The all-hosts pause is the BFF's own breaker being open. A warm is
		// the least urgent thing in the system; it does not get to be the
		// request that finds out whether the breaker has closed.
		if (this.globalPauseRemaining() > 0) return [];
		if (Date.now() - this.lastWarmAt < WARM_MIN_INTERVAL_MS) return [];

		const ladderHosts = new Set<string>();
		for (const [key] of windowOrder.slice(0, LADDER_OWNED_AHEAD)) {
			const host = hostOf(key);
			if (host !== null) ladderHosts.add(host);
		}

		const urls: string[] = [];
		for (const [key] of windowOrder.slice(LADDER_OWNED_AHEAD)) {
			if (urls.length >= MAX_WARM_URLS) break;
			if (this.warmAttempted.has(key)) continue;
			if (this.hasBodyOrPending(key)) continue;

			const host = hostOf(key);
			if (host === null) continue;
			// Never the same host as something the ladder is going to fetch:
			// that is the crawl-delay collision the split exists to avoid.
			if (ladderHosts.has(host)) continue;
			if (this.hostPauseRemaining(host) > 0) continue;
			if (this.hostInflight.has(host)) continue;

			// Deliberately no per-host dedupe among these. The server takes at
			// most one URL per host per call and reports the rest as
			// skipped_same_host; duplicating that here would be a second copy
			// of a rule only one side can enforce correctly.
			urls.push(key);
		}
		return urls;
	}

	/**
	 * Fire the warm. Never awaited, never surfaced, never retried.
	 *
	 * The ledger writes happen before the call, not after: from this moment
	 * the ladder must not touch these URLs, and a promise that settles a tick
	 * later is too late to decide that.
	 */
	private issueWarm(urls: string[]): void {
		this.lastWarmAt = Date.now();
		const settleUntil = this.lastWarmAt + WARM_SETTLE_MS;
		for (const url of urls) {
			this.rememberWarm(url, settleUntil);
		}

		void batchPrefetchArticleContentClient(urls).then(
			() => undefined,
			(error: unknown) => this.onWarmRefused(urls, error),
		);
	}

	/**
	 * A warm that never happened. The articles go back to the per-URL ladder
	 * immediately — the whole point of keeping the two paths independent is
	 * that this path failing costs pacing and nothing else.
	 *
	 * No cooldown is armed from here under any code. A cooldown suppresses the
	 * requests a reader is waiting on, and nobody was waiting on this one; a
	 * warm refused by a saturated pool says nothing about the publisher.
	 */
	private onWarmRefused(urls: string[], error: unknown): void {
		for (const url of urls) this.warmSettleUntil.delete(url);

		const connectErr = ConnectError.from(error);
		if (connectErr.code === Code.FailedPrecondition) {
			// ADR-000966's shape: a capability that is off names itself rather
			// than failing as something transient. Latched, said once, and
			// never retried — no configuration changes because we asked twice.
			if (!this.warmDisabled) {
				this.warmDisabled = true;
				console.info(
					"[ArticlePrefetcher] server-side article content warm is disabled; per-URL prefetch continues",
					connectErr.message,
				);
			}
			return;
		}

		console.debug(
			"[ArticlePrefetcher] article content warm refused",
			connectErr.message,
		);
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

		const host = hostOf(cacheKey);
		if (host === null) return Promise.resolve();

		// Honor the cooldown outright. The lookahead has nobody waiting on it,
		// so unlike the visible card it gets no probe: skip until it lifts.
		if (this.hostPauseRemaining(host) > 0) return Promise.resolve();

		// Serialize per-host: chain after any in-flight prefetch on this host.
		const previous = this.hostInflight.get(host) ?? Promise.resolve();
		const next = previous.then(() =>
			this.fetchContent(cacheKey, host, false, "low").then(() => undefined),
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

		const host = hostOf(feedUrl);
		if (host === null) return Promise.resolve(null);

		// A cooldown covering this card does not silence it. The reader asked
		// for this one body, so they get an attempt — rationed, not refused.
		const scope = this.activeCooldownScope(host);
		if (scope !== null && !this.claimForegroundProbe(scope)) {
			return Promise.resolve(null);
		}

		// The visible card does not queue behind the lookahead prefetches — it
		// is what the reader is waiting on. It still registers on the host chain
		// so those prefetches fall in behind it instead of racing it.
		const run = this.fetchContent(feedUrl, host, scope !== null, "high");
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

	/**
	 * Single in-flight request per URL, shared by prefetch and visible card.
	 *
	 * The priority travels with whoever started the request. A visible card
	 * that joins a lookahead already in flight inherits that lookahead's low
	 * lane, which is the same trade the join itself makes: one late request
	 * beats two that fight each other through the host rate limiter.
	 */
	private fetchContent(
		cacheKey: string,
		host: string,
		crossCooldown = false,
		priority: AltFetchPriority = "low",
	): Promise<string | null> {
		const pending = this.inflight.get(cacheKey);
		if (pending) return pending;

		const run = this.runFetch(cacheKey, host, crossCooldown, priority);
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
		priority: AltFetchPriority,
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

			const response = await getFeedContentOnTheFlyClient(cacheKey, {
				priority,
			});
			this.clearCooldowns(host);
			// It came back. Nothing is owed on this article any more.
			this.retryLedger.delete(cacheKey);

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

			// The proxy URL alone. `og_image_url` is the publisher's own URL and
			// is deliberately not a fallback: an article whose image the signer
			// could not or would not sign has no thumbnail on this surface.
			this.ogImageCache.set(cacheKey, response.og_image_proxy_url || null);
			this.onOgImageFetched?.();

			this.evictOldEntries();
			return response.content || null;
		} catch (error) {
			this.contentCache.delete(cacheKey);
			const connectErr = ConnectError.from(error);
			const retryAfterMs = this.clampRetryAfter(
				parseRetryAfter(connectErr.metadata.get("Retry-After")),
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
			// Book the retry after the cooldowns above, so the delay is
			// computed against the gate this failure just shut.
			this.armRetry(cacheKey, host, connectErr, retryAfterMs);
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
		const wanted = new Map<string, RenderFeed>();
		for (let i = 1; i <= prefetchAhead; i++) {
			const feed = feeds[activeIndex + i];
			const key = feed?.normalizedUrl;
			if (feed && key) wanted.set(key, feed);
		}
		this.windowFeeds = wanted;

		// Hand the window's far slots to the server before scheduling the
		// ladder, so a URL that becomes warm-owned on this pass is already
		// marked when schedulePrefetch decides what to do with it.
		const warmUrls = this.selectWarmUrls([...wanted]);
		if (warmUrls.length > 0) this.issueWarm(warmUrls);

		// Cancel only the timers whose feed left the lookahead window. Clearing
		// every timer on each call restarted the 500ms ladder from zero, and the
		// caller re-runs this more often than that, so the far slots never fired.
		for (const [key, timeout] of this.prefetchTimeouts) {
			if (!wanted.has(key)) {
				clearTimeout(timeout);
				this.prefetchTimeouts.delete(key);
			}
		}

		// A booked retry belongs to the window, not to the article. Once the
		// reader has swiped past it, re-fetching would spend a publisher's
		// budget on a card nobody is heading towards any more.
		for (const key of [...this.retryLedger.keys()]) {
			if (!wanted.has(key)) this.retryLedger.delete(key);
		}

		// Hand the early rungs to the hosts that can actually use them.
		// Lookahead position was the only priority signal, so an article whose
		// host was cooling down or already busy still held the 500ms slot and
		// the one behind it — reachable right now — waited out a delay for a
		// request that the gate was going to refuse to send anyway.
		//
		// This is a permutation, not an addition: the same articles, the same
		// rung spacing, the same per-host serialization and cooldowns behind
		// it. prefetchAhead is untouched — ADR-000884 cut it 10 -> 4 after a
		// production 429 incident and nothing here reopens that.
		const reachable: [string, RenderFeed][] = [];
		const blocked: [string, RenderFeed][] = [];
		for (const entry of wanted) {
			const host = hostOf(entry[0]);
			const free =
				host !== null &&
				this.hostPauseRemaining(host) === 0 &&
				!this.hostInflight.has(host);
			(free ? reachable : blocked).push(entry);
		}
		const ordered = [...reachable, ...blocked];

		// The rung counter walks every window slot, including the ones already
		// cached or already scheduled, so the N-th article still waits N
		// PREFETCH_DELAYs. Compacting it would quietly tighten the pacing of a
		// partly-cached window.
		let rung = 0;
		for (const [cacheKey, nextFeed] of ordered) {
			rung += 1;
			if (this.prefetchTimeouts.has(cacheKey)) continue;
			if (this.hasBodyOrPending(cacheKey)) continue;

			this.schedulePrefetch(nextFeed, cacheKey, PREFETCH_DELAY * rung);
		}
	}

	private schedulePrefetch(
		feed: RenderFeed,
		cacheKey: string,
		delay: number,
	): void {
		// One timer per article. A retry booked while a ladder timer is still
		// pending for the same key would otherwise orphan that timer, which
		// still fires and issues the request the booking was meant to replace.
		const pending = this.prefetchTimeouts.get(cacheKey);
		if (pending) clearTimeout(pending);

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

			// An article the server is warming right now must not also be
			// fetched per-URL: the two meet at alt-backend's shared
			// crawl-delay gate and the loser comes back as a 429. Re-arm
			// rather than drop — after the settle window the warm has either
			// stored the body, making this a database read, or given up,
			// leaving this the one real fetch. The publisher sees one request
			// either way.
			const settle = this.warmSettleRemaining(cacheKey);
			if (settle > 0) {
				this.schedulePrefetch(feed, cacheKey, settle);
				return;
			}

			// Same for a booked retry that a host cooldown has overtaken since
			// it was scheduled. prefetchContent would drop it silently, and the
			// attempt would be spent on a request that never left the browser.
			if (this.retryLedger.has(cacheKey)) {
				const host = hostOf(cacheKey);
				const hostPause = host === null ? 0 : this.hostPauseRemaining(host);
				if (hostPause > 0) {
					const next = hostPause + PREFETCH_DELAY;
					const entry = this.retryLedger.get(cacheKey);
					if (entry) entry.nextAt = Date.now() + next;
					this.schedulePrefetch(feed, cacheKey, next);
					return;
				}
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
	 * The signed og:image proxy path for a feed URL, or null when there is
	 * none. See `ogImageCache`: this is never a publisher URL.
	 */
	public getCachedOgImage(feedUrl: string): string | null {
		return this.ogImageCache.get(feedUrl) ?? null;
	}

	/**
	 * Seed cache directly without fetching from API.
	 * Used by SwipeFeedScreen to cache the first feed's content from loadMore.
	 *
	 * `ogImageProxyUrl` is a signed `/v1/images/proxy/...` path or null. There
	 * used to be a second slot here for the publisher's own URL, taken only
	 * when no proxy URL was supplied — and every caller that had a raw URL and
	 * no proxy one duly filled it, which is how an unproxied cross-origin URL
	 * reached the swipe card's `<img src>`. One slot, and it is the signed one.
	 */
	public seedCache(
		feedUrl: string,
		content: string,
		articleId: string,
		ogImageProxyUrl: string | null,
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
		this.ogImageCache.set(feedUrl, ogImageProxyUrl || null);
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
