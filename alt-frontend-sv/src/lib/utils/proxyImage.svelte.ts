/**
 * Viewport-gated, status-aware OG image loading as a reusable rune.
 *
 * This is the four-state pipeline the visual feed surfaces share:
 *
 *   idle -> loading -> loaded | absent
 *
 * Two properties are the whole reason it exists as a unit rather than as
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
 *
 * Must be called during component initialisation — it registers `$effect`s and
 * an `onDestroy` hook against the calling component's lifecycle.
 */
import { onDestroy } from "svelte";
import { loadProxyImageDefault } from "./loadProxyImage";

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
}

export interface ProxyImage {
	readonly state: ProxyImageState;
	/** The object URL to render, once `state` is `"loaded"`. */
	readonly objectUrl: string | null;
}

export function createProxyImage(options: ProxyImageOptions): ProxyImage {
	const load = options.load ?? loadProxyImageDefault;
	const rootMargin = options.rootMargin ?? "200px";

	let state = $state<ProxyImageState>("idle");
	let objectUrl = $state<string | null>(null);
	let inView = $state(false);

	// Non-reactive bookkeeping: these drive control flow inside the effects and
	// must not re-trigger them.
	let trackedUrl: string | null = null;
	let revokeUrl: string | null = null;
	let loadStartedForUrl: string | null = null;
	let abortController: AbortController | null = null;

	function reset() {
		abortController?.abort();
		abortController = null;
		if (revokeUrl) {
			URL.revokeObjectURL(revokeUrl);
			revokeUrl = null;
		}
		objectUrl = null;
		loadStartedForUrl = null;
		state = "idle";
	}

	// Restart when the URL changes (raw -> proxy backfill, or a recycled card
	// being handed a different feed). Ordered before the load effect below so a
	// URL change resets first and re-loads second, within the same flush.
	$effect(() => {
		const url = options.url() ?? null;
		if (url !== trackedUrl) {
			trackedUrl = url;
			reset();
		}
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

	$effect(() => {
		const url = options.url();
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
				if (revokeUrl) URL.revokeObjectURL(revokeUrl);
				revokeUrl = result.objectUrl;
				objectUrl = result.objectUrl;
				state = "loaded";
			} else {
				state = "absent";
			}
		});
	});

	onDestroy(() => {
		abortController?.abort();
		if (revokeUrl) URL.revokeObjectURL(revokeUrl);
	});

	return {
		get state() {
			return state;
		},
		get objectUrl() {
			return objectUrl;
		},
	};
}
