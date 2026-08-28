/**
 * Viewport-gated, status-aware OG image loading as a reusable rune.
 *
 * This is the four-state pipeline the visual feed surfaces share:
 *
 *   idle -> loading -> loaded | absent
 *
 * Three properties are the whole reason it exists as a unit rather than as
 * per-component code:
 *
 *  - **Viewport gating.** The image proxy rate-limits per upstream host, so a
 *    grid of same-host thumbnails will exhaust its budget if every card fetches
 *    on mount. Only cards the reader can actually reach start a load.
 *  - **"Loading" is not "absent".** A transient 429 is retried *inside*
 *    `loadProxyImage`, and the caller must keep showing a shimmer while that
 *    happens. Collapsing to the fallback on the first failure is the regression
 *    that made mark-as-read blank out surviving cards on the desktop grid; the
 *    `absent` state is reached only once the loader has given up for good.
 *  - **Resolution finishes after we stop listening.** `ResolveOgImages` fetches
 *    the publisher's page inline behind a per-host slot, so a batch that cannot
 *    finish inside one RPC leaves the reader's request dead and the images it
 *    did resolve sitting in the store. A card that treated that as the origin's
 *    answer stayed blank until the page was reloaded. It now re-asks, on a
 *    short bounded ladder, and the second ask is answered from the store with
 *    no origin request at all — which is what makes the grid fill itself in.
 *
 * Must be called during component initialisation — it registers `$effect`s and
 * an `onDestroy` hook against the calling component's lifecycle.
 */
import { onDestroy } from "svelte";
import { loadProxyImageDefault } from "./loadProxyImage";
import type { OgImageOutcome } from "./ogImageResolver";
import { MAX_RESOLVE_ATTEMPTS, ogImageRetryDelayMs } from "./ogImageRetry";

export type ProxyImageState = "idle" | "loading" | "loaded" | "absent";

export interface ProxyImageOptions {
	/** Reactive getter for the proxy URL. A change restarts the pipeline. */
	url: () => string | null | undefined;
	/** Reactive getter for the element whose viewport entry gates the load. */
	container: () => HTMLElement | null;
	/** How early to start loading relative to the viewport. */
	rootMargin?: string;
	/** Injectable loader, for tests. */
	load?: typeof loadProxyImageDefault;
	/**
	 * Obtains a proxy URL for a feed that arrived without one, called only
	 * after the card has actually entered the viewport.
	 *
	 * This is what makes resolution on demand rather than a crawl: the request
	 * exists because a reader reached this card. Omit it on surfaces that have
	 * no feed to resolve, and a card with no URL settles straight to `absent`.
	 */
	resolve?: () => Promise<OgImageOutcome>;
}

export interface ProxyImage {
	readonly state: ProxyImageState;
	/**
	 * The URL to render, once `state` is `"loaded"`.
	 *
	 * This is the proxy URL the load actually succeeded on, captured at that
	 * moment — not a live read of the current URL, which may be one nothing has
	 * probed yet or one a superseded feed left behind. Rendering it directly is
	 * what lets a preload hint, `loading`, `fetchpriority` and `decoding` apply
	 * to the same string the `<img>` ends up requesting.
	 */
	readonly src: string | null;
}

export function createProxyImage(options: ProxyImageOptions): ProxyImage {
	const load = options.load ?? loadProxyImageDefault;
	const rootMargin = options.rootMargin ?? "200px";

	let state = $state<ProxyImageState>("idle");
	// The URL a load succeeded on, and the only one the card may render.
	let src = $state<string | null>(null);
	let inView = $state(false);
	// A URL obtained on demand, once the card was actually reached.
	let resolvedUrl = $state<string | null>(null);

	// Non-reactive bookkeeping: these drive control flow inside the effects and
	// must not re-trigger them.
	let trackedUrl: string | null = null;
	let loadStartedForUrl: string | null = null;
	let abortController: AbortController | null = null;
	let resolveStarted = false;
	let retryTimer: ReturnType<typeof setTimeout> | null = null;
	let destroyed = false;

	/**
	 * The URL actually loaded.
	 *
	 * A URL we resolved on demand outranks one the feed list hands us later.
	 * Both name the same publisher's image, so preferring the newcomer would
	 * only swap `src` for a different-but-equivalent URL — and changing an
	 * `<img>`'s src restarts its load, flashing the card back to a shimmer to
	 * re-fetch a picture already on screen. `reset()` clears `resolvedUrl`, so
	 * a card genuinely handed a different feed still follows the feed's own
	 * URL.
	 */
	const effectiveUrl = () => resolvedUrl || options.url() || null;

	function clearRetry() {
		if (retryTimer !== null) {
			clearTimeout(retryTimer);
			retryTimer = null;
		}
	}

	function reset() {
		abortController?.abort();
		abortController = null;
		clearRetry();
		src = null;
		loadStartedForUrl = null;
		state = "idle";
		resolvedUrl = null;
		resolveStarted = false;
	}

	/**
	 * Ask for this feed's image, and keep asking while the answer is that we
	 * could not ask.
	 *
	 * Only `unavailable` walks the ladder. `absent` is the origin's own answer,
	 * recorded server-side, and re-asking it would cost the publisher a request
	 * to be told the same thing.
	 */
	function startResolve(attempt: number) {
		if (destroyed || !options.resolve) return;

		state = "loading";
		options
			.resolve()
			.then((outcome) => {
				if (destroyed) return;
				if (outcome.status === "resolved") {
					resolvedUrl = outcome.url;
					return;
				}
				if (outcome.status === "absent") {
					state = "absent";
					return;
				}

				const next = attempt + 1;
				if (next >= MAX_RESOLVE_ATTEMPTS) {
					// Out of asks. The card says "no preview" rather than
					// shimmering for the rest of the session.
					state = "absent";
					return;
				}
				clearRetry();
				retryTimer = setTimeout(
					() => {
						retryTimer = null;
						startResolve(next);
					},
					ogImageRetryDelayMs(attempt, outcome.retryAfterMs),
				);
			})
			.catch(() => {
				if (!destroyed) state = "absent";
			});
	}

	// Restart when the card is handed a *different* feed's URL.
	//
	// A first URL arriving where there was none is not that: it is the same
	// feed's image being backfilled, and resetting there would throw away a
	// resolution already in flight or an image already painted. Ordered before
	// the load effect below so a genuine change resets first and re-loads
	// second, within the same flush.
	$effect(() => {
		const url = options.url() ?? null;
		if (url === trackedUrl) return;

		const isBackfill = trackedUrl === null;
		trackedUrl = url;
		if (!isBackfill) reset();
	});

	// Observe viewport entry once; `inView` latches so scrolling back and forth
	// does not re-enter the pipeline.
	$effect(() => {
		const el = options.container();
		if (!el || inView) return;

		const io = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting)) {
					inView = true;
					io.disconnect();
				}
			},
			{ rootMargin },
		);
		io.observe(el);
		return () => io.disconnect();
	});

	// Resolve on demand: the card is in view and the feed carried no URL.
	$effect(() => {
		if (!inView || resolveStarted || options.url()) return;

		if (!options.resolve) {
			// Nothing can produce a URL for this card, so stop showing a
			// shimmer that will never end.
			state = "absent";
			return;
		}

		resolveStarted = true;
		startResolve(0);
	});

	$effect(() => {
		const url = effectiveUrl();
		if (!url || !inView || loadStartedForUrl === url) return;

		loadStartedForUrl = url;
		state = "loading";
		const ac = new AbortController();
		abortController = ac;

		load(url, ac.signal).then((result) => {
			// A superseded load must not paint into the card that has since been
			// handed a different feed.
			if (ac.signal.aborted) return;
			if (result.status === "loaded") {
				// The captured `url`, never a fresh `effectiveUrl()`: only the URL
				// this load actually probed has been shown to answer 200.
				src = url;
				state = "loaded";
			} else {
				state = "absent";
			}
		});
	});

	onDestroy(() => {
		destroyed = true;
		clearRetry();
		abortController?.abort();
	});

	return {
		get state() {
			return state;
		},
		get src() {
			return src;
		},
	};
}
